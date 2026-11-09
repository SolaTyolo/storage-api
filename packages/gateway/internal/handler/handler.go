package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/SolaTyolo/storage-api/internal/adapter"
	"github.com/SolaTyolo/storage-api/internal/auth"
	"github.com/SolaTyolo/storage-api/internal/config"
)

const (
	// S3 limits.
	maxKeyLength      = 1024
	maxPartNumber     = 10000
	maxListKeys       = 1000
	maxPresignBatch   = 1000
	readyCheckTimeout = 5 * time.Second
)

// Handler serves the gateway HTTP API.
type Handler struct {
	cfg      *config.Config
	registry *adapter.Registry
	auth     *auth.Authenticator
	log      *slog.Logger
}

// New builds a Handler.
func New(cfg *config.Config, registry *adapter.Registry, authenticator *auth.Authenticator, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{cfg: cfg, registry: registry, auth: authenticator, log: log}
}

// Router returns the chi router.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(h.accessLog)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   h.cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/ready", h.ready)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(h.limitBody)
		api.Use(h.auth.Middleware)
		api.Post("/presign/put", handle(h, h.presignPut))
		api.Post("/presign/get", handle(h, h.presignGet))
		api.Post("/multipart/create", handle(h, h.multipartCreate))
		api.Post("/multipart/presign-part", handle(h, h.multipartPresignPart))
		api.Post("/multipart/presign-parts", handle(h, h.multipartPresignParts))
		api.Post("/multipart/complete", handle(h, h.multipartComplete))
		api.Post("/multipart/abort", handle(h, h.multipartAbort))
		api.Post("/object/head", handle(h, h.objectHead))
		api.Post("/object/list", handle(h, h.objectList))
		api.Post("/object/delete", handle(h, h.objectDelete))
		api.Post("/object/copy", handle(h, h.objectCopy))
	})

	return r
}

// handle decodes a JSON body into T, runs fn, and writes the JSON result or error.
func handle[T any](h *Handler, fn func(r *http.Request, req *T) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req T
		if err := decodeJSON(r, &req); err != nil {
			h.writeErr(w, r, err)
			return
		}
		out, err := fn(r, &req)
		if err != nil {
			h.writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
	defer cancel()
	ok := true
	for _, name := range h.registry.Buckets() {
		ad, err := h.registry.Get(name)
		if err == nil {
			err = ad.Ping(ctx, name)
		}
		if err != nil {
			ok = false
			h.logger(r).Warn("readiness check failed", "bucket", name, "error", err)
		}
	}
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if r.URL.Path == "/health" {
			return
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		h.logger(r).Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

func (h *Handler) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) logger(r *http.Request) *slog.Logger {
	if id := middleware.GetReqID(r.Context()); id != "" {
		return h.log.With("request_id", id)
	}
	return h.log
}

func (h *Handler) expire(sec int) (time.Duration, error) {
	if sec < 0 {
		return 0, errBadRequest("expireSec must be >= 0")
	}
	if sec == 0 {
		return h.cfg.DefaultExpire, nil
	}
	d := time.Duration(sec) * time.Second
	if d > h.cfg.MaxExpire {
		return 0, errBadRequest(fmt.Sprintf("expireSec must be <= %d", int(h.cfg.MaxExpire.Seconds())))
	}
	return d, nil
}

func (h *Handler) resolve(r *http.Request, bucket, key string, write bool) (adapter.ObjectAdapter, error) {
	claims := auth.ClaimsFromContext(r.Context())
	if err := auth.AuthorizeBucketKey(claims, bucket, key, write); err != nil {
		return nil, errForbidden(err.Error())
	}
	ad, err := h.registry.Get(bucket)
	if err != nil {
		return nil, errBadRequest(err.Error())
	}
	return ad, nil
}

// checkUpload enforces bucket upload policy and returns the size to sign (-1 = unsigned).
func (h *Handler) checkUpload(bucket, contentType string, size *int64) (int64, error) {
	bc, _ := h.cfg.GetBucket(bucket)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !bc.ContentTypeAllowed(contentType) {
		return 0, &apiError{status: http.StatusUnsupportedMediaType, code: "unsupported_media_type",
			message: fmt.Sprintf("content type %q is not allowed for this bucket", contentType)}
	}
	signed := int64(-1)
	if size != nil {
		if *size < 0 {
			return 0, errBadRequest("size must be >= 0")
		}
		signed = *size
	}
	if bc.MaxUploadBytes > 0 {
		if size == nil {
			return 0, errBadRequest("size is required for this bucket")
		}
		if *size > bc.MaxUploadBytes {
			return 0, errTooLarge(bc.MaxUploadBytes)
		}
	}
	return signed, nil
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return e.message }

func errBadRequest(msg string) error {
	return &apiError{status: http.StatusBadRequest, code: "bad_request", message: msg}
}

func errForbidden(msg string) error {
	return &apiError{status: http.StatusForbidden, code: "forbidden", message: msg}
}

func errTooLarge(limit int64) error {
	return &apiError{status: http.StatusRequestEntityTooLarge, code: "payload_too_large",
		message: fmt.Sprintf("object exceeds maxUploadBytes (%d)", limit)}
}

// toAPIError maps adapter and transport errors to client-safe API errors.
func toAPIError(err error) *apiError {
	var ae *apiError
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &ae):
		return ae
	case errors.As(err, &maxBytes):
		return &apiError{status: http.StatusRequestEntityTooLarge, code: "payload_too_large", message: "request body too large"}
	case errors.Is(err, adapter.ErrNotFound):
		return &apiError{status: http.StatusNotFound, code: "not_found", message: "not found"}
	case errors.Is(err, adapter.ErrInvalid):
		return &apiError{status: http.StatusBadRequest, code: "bad_request", message: err.Error()}
	case errors.Is(err, adapter.ErrPrecondition):
		return &apiError{status: http.StatusPreconditionFailed, code: "precondition_failed", message: "precondition failed"}
	case errors.Is(err, adapter.ErrAccessDenied):
		return &apiError{status: http.StatusBadGateway, code: "storage_error", message: "storage rejected the gateway credentials"}
	case errors.Is(err, context.DeadlineExceeded):
		return &apiError{status: http.StatusGatewayTimeout, code: "timeout", message: "storage request timed out"}
	default:
		return &apiError{status: http.StatusInternalServerError, code: "internal", message: "internal error"}
	}
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	ae := toAPIError(err)
	if ae.status >= http.StatusInternalServerError {
		h.logger(r).Error("request failed", "path", r.URL.Path, "status", ae.status, "error", err)
	}
	body := map[string]string{"error": ae.code, "message": ae.message}
	if id := middleware.GetReqID(r.Context()); id != "" && ae.status >= http.StatusInternalServerError {
		body["requestId"] = id
	}
	writeJSON(w, ae.status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			return err
		}
		return errBadRequest("invalid json: " + err.Error())
	}
	return nil
}

func requireBucket(bucket string) error {
	if strings.TrimSpace(bucket) == "" {
		return errBadRequest("bucket is required")
	}
	return nil
}

func requireBucketKey(bucket, key string) error {
	if err := requireBucket(bucket); err != nil {
		return err
	}
	if key == "" {
		return errBadRequest("key is required")
	}
	return validateKey(key)
}

// validateKey rejects keys that CDNs or browsers could normalize into another
// path (".", "..", "//"), leading slashes, and control characters.
// A single trailing "/" is allowed for folder markers.
func validateKey(key string) error {
	if len(key) > maxKeyLength {
		return errBadRequest(fmt.Sprintf("key must be at most %d bytes", maxKeyLength))
	}
	if !utf8.ValidString(key) {
		return errBadRequest("key must be valid UTF-8")
	}
	for _, c := range key {
		if c < 0x20 || c == 0x7f {
			return errBadRequest("key must not contain control characters")
		}
	}
	if strings.HasPrefix(key, "/") {
		return errBadRequest("key must not start with '/'")
	}
	for _, seg := range strings.Split(strings.TrimSuffix(key, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return errBadRequest("key must not contain empty, '.' or '..' path segments")
		}
	}
	return nil
}
