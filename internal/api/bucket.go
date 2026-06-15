package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/model"
)

func (h *Handler) listBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.registry.ListAllBuckets(r.Context())
	if err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]model.Bucket, 0, len(buckets))
	now := time.Now().UTC()
	for _, b := range buckets {
		out = append(out, model.Bucket{
			ID:               b.DisplayID,
			Name:             b.Bucket,
			Public:           b.Public,
			FileSizeLimit:    b.FileSizeLimit,
			AllowedMimeTypes: b.AllowedMimeTypes,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type createBucketReq struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Public           bool     `json:"public"`
	FileSizeLimit    *int64   `json:"file_size_limit"`
	AllowedMimeTypes []string `json:"allowed_mime_types"`
	Type             string   `json:"type"`
}

func (h *Handler) createBucket(w http.ResponseWriter, r *http.Request) {
	var req createBucketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	if req.ID == "" {
		req.ID = req.Name
	}
	if req.ID == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	resolved, eng, err := h.registry.Resolve(r.Context(), req.ID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	meta := engine.BucketMeta{
		Public:           req.Public,
		FileSizeLimit:    req.FileSizeLimit,
		AllowedMimeTypes: req.AllowedMimeTypes,
	}
	if err := eng.CreateBucket(r.Context(), resolved.Bucket, meta); err != nil {
		h.logError(r, "bucket create failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "bucket created", "engine", resolved.Engine, "bucket", resolved.Bucket, "display_id", resolved.DisplayID, "public", req.Public)
	writeJSON(w, http.StatusOK, map[string]string{"name": resolved.DisplayID})
}

func (h *Handler) getBucket(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketId")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.HeadBucket(r.Context(), resolved.Bucket); err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "bucket not found")
			return
		}
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	meta, etag, _ := eng.GetBucketMeta(r.Context(), resolved.Bucket)
	now := time.Now().UTC()
	if etag != "" {
		w.Header().Set("ETag", formatETag(etag))
	}
	writeJSON(w, http.StatusOK, model.Bucket{
		ID:               resolved.DisplayID,
		Name:             resolved.Bucket,
		Public:           meta.Public,
		FileSizeLimit:    meta.FileSizeLimit,
		AllowedMimeTypes: meta.AllowedMimeTypes,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (h *Handler) updateBucket(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketId")
	var req createBucketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.HeadBucket(r.Context(), resolved.Bucket); err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "bucket not found")
			return
		}
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	meta := engine.BucketMeta{
		Public:           req.Public,
		FileSizeLimit:    req.FileSizeLimit,
		AllowedMimeTypes: req.AllowedMimeTypes,
	}
	if err := eng.SetBucketMeta(r.Context(), resolved.Bucket, meta, normalizeIfMatch(r.Header.Get("If-Match"))); err != nil {
		if errors.Is(err, engine.ErrPreconditionFailed) {
			writeStorageErr(w, http.StatusPreconditionFailed, "precondition_failed", "bucket metadata was modified concurrently")
			return
		}
		h.logError(r, "bucket update failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "bucket updated", "engine", resolved.Engine, "bucket", resolved.Bucket, "display_id", resolved.DisplayID, "public", req.Public)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully updated"})
}

func (h *Handler) deleteBucket(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketId")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.DeleteBucket(r.Context(), resolved.Bucket); err != nil {
		h.logError(r, "bucket delete failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "bucket deleted", "engine", resolved.Engine, "bucket", resolved.Bucket, "display_id", resolved.DisplayID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully deleted"})
}

func (h *Handler) emptyBucket(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketId")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.EmptyBucket(r.Context(), resolved.Bucket); err != nil {
		h.logError(r, "bucket empty failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "bucket emptied", "engine", resolved.Engine, "bucket", resolved.Bucket, "display_id", resolved.DisplayID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully emptied"})
}

func engineIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NotFound") || strings.Contains(msg, "NoSuchBucket")
}

func formatETag(etag string) string {
	if strings.HasPrefix(etag, `"`) {
		return etag
	}
	return `"` + etag + `"`
}

func normalizeIfMatch(v string) string {
	return strings.Trim(v, `"`)
}
