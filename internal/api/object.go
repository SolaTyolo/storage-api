package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/model"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) uploadObject(w http.ResponseWriter, r *http.Request) {
	h.putObject(w, r, false)
}

func (h *Handler) updateObject(w http.ResponseWriter, r *http.Request) {
	h.putObject(w, r, true)
}

func (h *Handler) putObject(w http.ResponseWriter, r *http.Request, isUpdate bool) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if objectPath == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "object path is required")
		return
	}

	body, contentType, cacheControl, metadata, err := readUploadBody(r)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	defer body.Close()

	if err := validateUploadPolicy(resolved, contentType, r.ContentLength); err != nil {
		if errors.Is(err, errFileTooLarge) {
			writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if resolved.FileSizeLimit != nil && *resolved.FileSizeLimit > 0 {
		body = limitUploadBody(body, *resolved.FileSizeLimit)
	}

	if !isUpdate && !upsertHeader(r) {
		if _, err := eng.HeadObject(r.Context(), resolved.Bucket, objectPath); err == nil {
			writeStorageErr(w, http.StatusConflict, "Duplicate", "The resource already exists")
			return
		}
	}

	if err := eng.PutObject(r.Context(), resolved.Bucket, objectPath, contentType, cacheControl, body, metadata); err != nil {
		if errors.Is(err, errFileTooLarge) {
			writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		h.logError(r, "object put failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	op := "upload"
	if isUpdate {
		op = "update"
	}
	h.logStorage(r, op, resolved.Engine, resolved.Bucket, objectPath, "display_id", resolved.DisplayID, "content_type", contentType)

	id := model.ObjectUUID(resolved.Engine, resolved.Bucket, objectPath).String()
	writeJSON(w, http.StatusOK, map[string]string{
		"Key": resolved.DisplayID + "/" + objectPath,
		"Id":  id,
	})
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request) {
	h.serveObject(w, r)
}

func (h *Handler) getPublicObject(w http.ResponseWriter, r *http.Request) {
	h.redirectPublicObject(w, r)
}

func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if strings.EqualFold(h.cfg.AuthDownloadMode, "redirect") {
		h.redirectPresignedGet(w, r, resolved, eng, objectPath, h.cfg.PresignExpires, "authenticated object redirect")
		return
	}

	body, info, err := eng.GetObject(r.Context(), resolved.Bucket, objectPath)
	if err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "object not found")
			return
		}
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer body.Close()

	ct := info.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if info.ETag != "" {
		w.Header().Set("ETag", `"`+info.ETag+`"`)
	}
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	if dl := r.URL.Query().Get("download"); dl != "" {
		name := dl
		if name == "" || name == "true" {
			name = filepath.Base(objectPath)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handler) headObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
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
	if info.ContentType != "" {
		w.Header().Set("Content-Type", info.ContentType)
	}
	if info.ETag != "" {
		w.Header().Set("ETag", `"`+info.ETag+`"`)
	}
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) objectInfo(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
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
	writeJSON(w, http.StatusOK, toFileObject(resolved, info))
}

func (h *Handler) deleteOneObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.DeleteObject(r.Context(), resolved.Bucket, objectPath); err != nil {
		h.logError(r, "object delete failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logStorage(r, "delete", resolved.Engine, resolved.Bucket, objectPath, "display_id", resolved.DisplayID)
	w.WriteHeader(http.StatusOK)
}

type deleteManyReq struct {
	Prefixes []string `json:"prefixes"`
}

func (h *Handler) deleteManyObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var req deleteManyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	if err := eng.DeleteObjects(r.Context(), resolved.Bucket, req.Prefixes); err != nil {
		h.logError(r, "objects delete failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "count", len(req.Prefixes), "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "objects deleted", "engine", resolved.Engine, "bucket", resolved.Bucket, "display_id", resolved.DisplayID, "count", len(req.Prefixes))
	out := make([]model.FileObject, 0, len(req.Prefixes))
	for _, p := range req.Prefixes {
		out = append(out, model.FileObject{Name: p})
	}
	writeJSON(w, http.StatusOK, out)
}

type listReq struct {
	Prefix string      `json:"prefix"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
	SortBy *sortBySpec `json:"sortBy"`
}

func (h *Handler) listObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var req listReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	objs, err := eng.ListObjects(r.Context(), resolved.Bucket, req.Prefix, req.Limit, req.Offset)
	if err != nil {
		h.logError(r, "object list failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logOp(r, slog.LevelDebug, "objects listed", "engine", resolved.Engine, "bucket", resolved.Bucket, "prefix", req.Prefix, "count", len(objs))
	out := make([]model.FileObject, 0, len(objs))
	for _, o := range objs {
		out = append(out, toFileObject(resolved, o))
	}
	sortFileObjects(out, req.SortBy)
	writeJSON(w, http.StatusOK, out)
}

type moveCopyReq struct {
	BucketID          string `json:"bucketId"`
	SourceKey         string `json:"sourceKey"`
	DestinationKey    string `json:"destinationKey"`
	DestinationBucket string `json:"destinationBucket"`
}

func (h *Handler) moveObject(w http.ResponseWriter, r *http.Request) {
	var req moveCopyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	src, srcEng, err := h.registry.Resolve(r.Context(), req.BucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	dstRef := req.DestinationBucket
	if dstRef == "" {
		dstRef = req.BucketID
	}
	dst, dstEng, err := h.registry.Resolve(r.Context(), dstRef)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := engine.TransferObject(r.Context(), srcEng, dstEng, src.Bucket, req.SourceKey, dst.Bucket, req.DestinationKey); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := srcEng.DeleteObject(r.Context(), src.Bucket, req.SourceKey); err != nil {
		h.logError(r, "move delete source failed", "engine", src.Engine, "bucket", src.Bucket, "path", req.SourceKey, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "object moved",
		"engine", src.Engine,
		"source_bucket", src.Bucket,
		"dest_bucket", dst.Bucket,
		"source_key", req.SourceKey,
		"dest_key", req.DestinationKey,
	)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully moved"})
}

func (h *Handler) copyObject(w http.ResponseWriter, r *http.Request) {
	var req moveCopyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	src, srcEng, err := h.registry.Resolve(r.Context(), req.BucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	dstRef := req.DestinationBucket
	if dstRef == "" {
		dstRef = req.BucketID
	}
	dst, dstEng, err := h.registry.Resolve(r.Context(), dstRef)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := engine.TransferObject(r.Context(), srcEng, dstEng, src.Bucket, req.SourceKey, dst.Bucket, req.DestinationKey); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	h.logInfo(r, "object copied",
		"engine", src.Engine,
		"source_bucket", src.Bucket,
		"dest_bucket", dst.Bucket,
		"source_key", req.SourceKey,
		"dest_key", req.DestinationKey,
	)
	writeJSON(w, http.StatusOK, map[string]string{"Key": req.DestinationKey})
}

type signReq struct {
	ExpiresIn int      `json:"expiresIn"`
	Paths     []string `json:"paths"`
}

func (h *Handler) signObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	var req signReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	expires := h.presignExpires(req.ExpiresIn)

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	signedURL, err := h.issuePresignedGet(r, eng, resolved.Bucket, objectPath, expires)
	if err != nil {
		h.logError(r, "download presign failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	h.logPresignedURLIssued(r, "download", resolved.Engine, resolved.Bucket, objectPath, signedURL, expires)
	writeJSON(w, http.StatusOK, map[string]string{"signedURL": signedURL, "path": objectPath})
}

func (h *Handler) signManyObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	var req signReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	expires := h.presignExpires(req.ExpiresIn)
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	out := make([]map[string]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		signedURL, err := eng.PresignGet(r.Context(), resolved.Bucket, p, expires, "")
		if err != nil {
			h.logError(r, "download presign failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", p, "error", err.Error())
			writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		h.logPresignedURLIssued(r, "download", resolved.Engine, resolved.Bucket, p, signedURL, expires)
		out = append(out, map[string]string{
			"path":      p,
			"signedURL": signedURL,
			"error":     "",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getSignedObject(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}
	h.logWarn(r, "deprecated legacy download sign token", "path", r.URL.Path)
	writeStorageErr(w, http.StatusGone, "deprecated", "legacy download sign tokens are no longer supported; use S3 presigned URL from signObject")
}

func (h *Handler) signUpload(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !h.cfg.AllowPresignedUpload {
		writeStorageErr(w, http.StatusForbidden, "access_denied", "presigned upload is disabled; use POST /object/{bucket}/{path}")
		return
	}

	expires := h.cfg.PresignExpires
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = r.URL.Query().Get("contentType")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := validateUploadPolicy(resolved, contentType, 0); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	signedURL, err := eng.PresignPut(r.Context(), resolved.Bucket, objectPath, contentType, expires)
	if err != nil {
		h.logError(r, "upload presign failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "path", objectPath, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	h.logPresignedURLIssued(r, "upload", resolved.Engine, resolved.Bucket, objectPath, signedURL, expires)
	writeJSON(w, http.StatusOK, map[string]string{
		"signedUrl": signedURL,
		"url":       signedURL,
		"path":      objectPath,
	})
}

func (h *Handler) uploadSigned(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}
	h.logWarn(r, "deprecated legacy upload sign token", "path", r.URL.Path)
	writeStorageErr(w, http.StatusGone, "deprecated", "legacy upload sign tokens are no longer supported; use S3 presigned PUT URL from signUpload")
}

func toFileObject(resolved engine.ResolvedBucket, info engine.ObjectInfo) model.FileObject {
	ts := info.LastModified
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	ct := info.ContentType
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(info.Path))
	}
	return model.FileObject{
		Name:           info.Path,
		ID:             model.ObjectUUID(resolved.Engine, resolved.Bucket, info.Path).String(),
		CreatedAt:      ts,
		UpdatedAt:      ts,
		LastAccessedAt: ts,
		Metadata: model.FileMetadata{
			ETag:     info.ETag,
			Size:     info.Size,
			MimeType: ct,
		},
	}
}

func readUploadBody(r *http.Request) (io.ReadCloser, string, string, map[string]string, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		return readMultipartUpload(r)
	}
	contentType := ct
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return io.NopCloser(r.Body), contentType, r.Header.Get("Cache-Control"), nil, nil
}

func readMultipartUpload(r *http.Request) (io.ReadCloser, string, string, map[string]string, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, "", "", nil, err
	}
	var filePart *multipart.Part
	var cacheControl string
	var metadata map[string]string
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", "", nil, err
		}
		name := part.FormName()
		switch name {
		case "", "file":
			if filePart == nil {
				filePart = part
			} else {
				part.Close()
			}
		case "cacheControl":
			b, _ := io.ReadAll(part)
			cacheControl = string(b)
			part.Close()
		case "metadata":
			b, _ := io.ReadAll(part)
			_ = json.Unmarshal(b, &metadata)
			part.Close()
		default:
			part.Close()
		}
	}
	if filePart == nil {
		return nil, "", "", nil, errors.New("file part missing")
	}
	contentType := filePart.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return filePart, contentType, cacheControl, metadata, nil
}
