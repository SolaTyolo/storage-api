package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/postship/storage/internal/config"
	"github.com/postship/storage/internal/mime"
	"github.com/postship/storage/internal/model"
	"github.com/postship/storage/internal/s3client"
	"github.com/postship/storage/internal/store"
	"github.com/postship/storage/internal/preview"
	"github.com/postship/storage/internal/transform"
	"github.com/postship/storage/internal/uploadtoken"
)

type Handler struct {
	cfg       config.Config
	store     *store.Store
	s3        *s3client.Client
	transform *transform.Service
	preview   *preview.Service
}

func NewRouter(cfg config.Config, st *store.Store, s3 *s3client.Client, tf *transform.Service, prev *preview.Service) http.Handler {
	h := &Handler{cfg: cfg, store: st, s3: s3, transform: tf, preview: prev}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", h.health)

	r.Get("/playground", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/playground/", http.StatusFound)
	})
	r.Handle("/playground/*", playgroundHandler())

	r.Route("/storage/v1", func(r chi.Router) {
		r.Get("/buckets", h.listBuckets)
		r.Post("/buckets", h.createBucket)
		r.Get("/buckets/{bucketID}", h.getBucket)

		r.Get("/buckets/{bucketID}/objects", h.listObjects)
		r.Get("/buckets/{bucketID}/objects/{objectName}", h.getObject)
		r.Delete("/buckets/{bucketID}/objects/{objectName}", h.deleteObject)

		r.Post("/buckets/{bucketID}/uploads/presign", h.presignUpload)
		r.Post("/buckets/{bucketID}/uploads/complete", h.completeUpload)

		r.Get("/objects/{objectID}/download-url", h.downloadURL)
		r.Get("/objects/{objectID}/image", h.serveImage)
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "db": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.store.ListBuckets(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, buckets)
}

type createBucketReq struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Public           bool     `json:"public"`
	FileSizeLimit    *int64   `json:"file_size_limit"`
	AllowedMimeTypes []string `json:"allowed_mime_types"`
}

func (h *Handler) createBucket(w http.ResponseWriter, r *http.Request) {
	var req createBucketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.ID == "" {
		req.ID = req.Name
	}
	if req.ID == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and name required"})
		return
	}
	b := model.Bucket{ID: req.ID, Name: req.Name, Public: req.Public, FileSizeLimit: req.FileSizeLimit, AllowedMimeTypes: req.AllowedMimeTypes}
	if err := h.store.CreateBucket(r.Context(), b); err != nil {
		writeErr(w, err)
		return
	}
	got, _ := h.store.GetBucket(r.Context(), b.ID)
	writeJSON(w, http.StatusCreated, got)
}

func (h *Handler) getBucket(w http.ResponseWriter, r *http.Request) {
	b, err := h.store.GetBucket(r.Context(), chi.URLParam(r, "bucketID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) listObjects(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	objs, err := h.store.ListObjects(r.Context(), chi.URLParam(r, "bucketID"), prefix, 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objs)
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "objectName")
	obj, err := h.store.GetObject(r.Context(), chi.URLParam(r, "bucketID"), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

func (h *Handler) deleteObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketID")
	name := chi.URLParam(r, "objectName")
	obj, err := h.store.GetObject(r.Context(), bucketID, name)
	if err != nil {
		writeErr(w, err)
		return
	}
	if obj.Metadata.S3Key != "" {
		_ = h.s3.Delete(r.Context(), obj.Metadata.S3Key)
	}
	if err := h.store.DeleteObject(r.Context(), bucketID, name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type presignReq struct {
	ObjectName  string `json:"object_name"`
	ContentType string `json:"content_type"`
}

func (h *Handler) presignUpload(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketID")
	if _, err := h.store.GetBucket(r.Context(), bucketID); err != nil {
		writeErr(w, err)
		return
	}

	var req presignReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.ObjectName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "object_name required"})
		return
	}
	ct := req.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	s3Key := s3client.ObjectKey(bucketID, req.ObjectName)
	url, err := h.s3.PresignPut(r.Context(), s3Key, ct, h.cfg.PresignExpires)
	if err != nil {
		writeErr(w, err)
		return
	}

	expires := time.Now().Add(h.cfg.PresignExpires)
	claims := uploadtoken.NewClaims(bucketID, req.ObjectName, s3Key, ct, expires)
	token, err := uploadtoken.Issue(h.cfg.UploadSigningSecret, claims)
	if err != nil {
		writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"presigned_url":   url,
		"complete_token":  token,
		"s3_key":          s3Key,
		"expires_in":      int(h.cfg.PresignExpires.Seconds()),
		"delivery": map[string]any{
			"transform_supported": mime.DeliverySupported(ct),
			"image_endpoint":      "/storage/v1/objects/{object_id}/image",
			"params": map[string]string{
				"w": "max width (px)",
				"h": "max height (px)",
				"c": "scale | fit | fill | pad | thumb",
				"q": "quality 1-100 (jpeg)",
				"f": "auto | jpg | png | webp",
				"t": "video frame time (seconds)",
				"page": "PDF/Office page (default 1)",
				"dpi":  "PDF render DPI (default 150)",
			},
		},
	})
}

type completeReq struct {
	CompleteToken string `json:"complete_token"`
}

func (h *Handler) completeUpload(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketID")

	var req completeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CompleteToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "complete_token required"})
		return
	}

	claims, err := uploadtoken.Verify(h.cfg.UploadSigningSecret, req.CompleteToken)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired complete_token"})
		return
	}
	if claims.BucketID != bucketID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token bucket mismatch"})
		return
	}

	if existing, err := h.store.GetObject(r.Context(), claims.BucketID, claims.ObjectName); err == nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	size, etag, err := h.s3.Head(r.Context(), claims.S3Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "object not found in s3, upload first"})
		return
	}

	obj := model.StorageObject{
		ID:       uuid.New(),
		BucketID: claims.BucketID,
		Name:     claims.ObjectName,
		Metadata: model.ObjectMetadata{
			MimeType: claims.ContentType,
			Size:     size,
			ETag:     etag,
			S3Key:    claims.S3Key,
		},
	}
	if err := h.store.UpsertObject(r.Context(), obj); err != nil {
		writeErr(w, err)
		return
	}

	got, _ := h.store.GetObject(r.Context(), claims.BucketID, claims.ObjectName)
	writeJSON(w, http.StatusOK, got)
}

func (h *Handler) downloadURL(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "objectID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid object id"})
		return
	}
	obj, err := h.store.GetObjectByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	url, err := h.s3.PresignGet(r.Context(), obj.Metadata.S3Key, h.cfg.PresignExpires)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url, "expires_in": int(h.cfg.PresignExpires.Seconds())})
}

func (h *Handler) serveImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "objectID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid object id"})
		return
	}
	obj, err := h.store.GetObjectByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}

	params, err := transform.ParseParams(r.URL.Query(), h.cfg.TransformMaxEdge)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	kind := mime.Classify(obj.Metadata.MimeType)

	// 图片可走 imgproxy（开源）；视频/PDF/Office 走专用管线
	if h.cfg.TransformBackend == "imgproxy" && kind == mime.KindImage {
		proxyURL, err := transform.ImgproxyURL(h.cfg, h.s3.Bucket(), obj.Metadata.S3Key, params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		http.Redirect(w, r, proxyURL, http.StatusFound)
		return
	}

	var body []byte
	var contentType string
	var renderErr error

	switch kind {
	case mime.KindPDF, mime.KindDocument:
		page, dpi := params.Page, params.DPI
		if page <= 0 {
			page = 1
		}
		if dpi <= 0 {
			dpi = 150
		}
		jpeg, rerr := h.preview.Rasterize(r.Context(), obj.Metadata.S3Key, obj.Metadata.MimeType, obj.Name, page, dpi)
		if errors.Is(rerr, preview.ErrNotSupported) {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": rerr.Error(), "kind": kind})
			return
		}
		if rerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": rerr.Error()})
			return
		}
		body, contentType, renderErr = h.transform.RenderJPEG(jpeg, params)
	default:
		body, contentType, renderErr = h.transform.Render(r.Context(), obj, params)
	}

	if errors.Is(renderErr, transform.ErrNotSupported) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"error": "delivery not supported for this media type",
			"kind":  kind,
		})
		return
	}
	if renderErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": renderErr.Error()})
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(body)
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

