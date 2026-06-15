package api

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/engine"
)

func (h *Handler) presignExpires(expiresInSec int) time.Duration {
	if expiresInSec > 0 {
		return time.Duration(expiresInSec) * time.Second
	}
	return h.cfg.PresignExpires
}

func (h *Handler) publicDownloadFilename(r *http.Request, objectPath string) string {
	dl := r.URL.Query().Get("download")
	if dl == "" {
		return ""
	}
	name := dl
	if name == "" || name == "true" {
		name = filepath.Base(objectPath)
	}
	return name
}

func (h *Handler) issuePresignedGet(r *http.Request, eng engine.Engine, bucket, objectPath string, expires time.Duration) (string, error) {
	filename := h.publicDownloadFilename(r, objectPath)
	return eng.PresignGet(r.Context(), bucket, objectPath, expires, filename)
}

func (h *Handler) redirectPresignedGet(w http.ResponseWriter, r *http.Request, resolved engine.ResolvedBucket, eng engine.Engine, objectPath string, expires time.Duration, logMsg string) {
	if _, err := eng.HeadObject(r.Context(), resolved.Bucket, objectPath); err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "object not found")
			return
		}
		h.logError(r, logMsg+" presign head failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	signedURL, err := h.issuePresignedGet(r, eng, resolved.Bucket, objectPath, expires)
	if err != nil {
		h.logError(r, logMsg+" presign failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	h.logInfo(r, logMsg,
		"engine", resolved.Engine,
		"bucket", resolved.Bucket,
		"path", objectPath,
		"expires_sec", int(expires.Seconds()),
	)
	http.Redirect(w, r, signedURL, http.StatusFound)
}

func (h *Handler) redirectPublicObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !resolved.Public {
		writeStorageErr(w, http.StatusForbidden, "access_denied", "bucket is not public")
		return
	}

	h.redirectPresignedGet(w, r, resolved, eng, objectPath, h.cfg.PublicPresignExpires, "public object redirect")
}

func (h *Handler) headPublicObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !resolved.Public {
		writeStorageErr(w, http.StatusForbidden, "access_denied", "bucket is not public")
		return
	}

	info, err := eng.HeadObject(r.Context(), resolved.Bucket, objectPath)
	if err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "object not found")
			return
		}
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	writeObjectHeadHeaders(w, info)
	w.WriteHeader(http.StatusOK)
}

func writeObjectHeadHeaders(w http.ResponseWriter, info engine.ObjectInfo) {
	if info.ContentType != "" {
		w.Header().Set("Content-Type", info.ContentType)
	}
	if info.ETag != "" {
		w.Header().Set("ETag", `"`+info.ETag+`"`)
	}
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
}

func (h *Handler) logPresignedURLIssued(r *http.Request, kind, engineName, bucket, objectPath, issuedURL string, expires time.Duration) {
	h.logInfo(r, "presigned URL issued",
		"kind", kind,
		"engine", engineName,
		"bucket", bucket,
		"path", objectPath,
		"expires_sec", int(expires.Seconds()),
		"url", issuedURL,
	)
}
