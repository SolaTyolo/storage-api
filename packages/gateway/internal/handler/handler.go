package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/SolaTyolo/storage-api/internal/adapter"
	"github.com/SolaTyolo/storage-api/internal/auth"
	"github.com/SolaTyolo/storage-api/internal/config"
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
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(h.auth.Middleware)
		api.Post("/presign/put", h.presignPut)
		api.Post("/presign/get", h.presignGet)
		api.Post("/multipart/create", h.multipartCreate)
		api.Post("/multipart/presign-part", h.multipartPresignPart)
		api.Post("/multipart/complete", h.multipartComplete)
		api.Post("/multipart/abort", h.multipartAbort)
		api.Post("/object/head", h.objectHead)
		api.Post("/object/list", h.objectList)
		api.Post("/object/delete", h.objectDelete)
		api.Post("/object/copy", h.objectCopy)
	})

	return r
}

func (h *Handler) expire(sec int) time.Duration {
	if sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return h.cfg.DefaultExpire
}

func (h *Handler) resolve(bucket, key string, write bool, r *http.Request) (adapter.ObjectAdapter, error) {
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

func errNotFound(msg string) error {
	return &apiError{status: http.StatusNotFound, code: "not_found", message: msg}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	if ae, ok := err.(*apiError); ok {
		writeJSON(w, ae.status, map[string]string{"error": ae.code, "message": ae.message})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "message": err.Error()})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return errBadRequest("invalid json: " + err.Error())
	}
	return nil
}
