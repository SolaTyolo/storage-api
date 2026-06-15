package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/model"
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

	resolved, eng, err := h.registry.Resolve(bucketID)
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

	if !isUpdate && !upsertHeader(r) {
		if _, err := eng.HeadObject(r.Context(), resolved.Bucket, objectPath); err == nil {
			writeStorageErr(w, http.StatusConflict, "Duplicate", "The resource already exists")
			return
		}
	}

	if err := eng.PutObject(r.Context(), resolved.Bucket, objectPath, contentType, body, metadata); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if cacheControl != "" {
		_ = cacheControl
	}

	id := model.ObjectUUID(resolved.Engine, resolved.Bucket, objectPath).String()
	writeJSON(w, http.StatusOK, map[string]string{
		"Key": resolved.DisplayID + "/" + objectPath,
		"Id":  id,
	})
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request) {
	h.serveObject(w, r, false)
}

func (h *Handler) getPublicObject(w http.ResponseWriter, r *http.Request) {
	h.serveObject(w, r, true)
}

func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request, requirePublic bool) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, eng, err := h.registry.Resolve(bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if requirePublic && !resolved.Public {
		writeStorageErr(w, http.StatusForbidden, "access_denied", "bucket is not public")
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

	resolved, eng, err := h.registry.Resolve(bucketID)
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

	resolved, eng, err := h.registry.Resolve(bucketID)
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

	resolved, eng, err := h.registry.Resolve(bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := eng.DeleteObject(r.Context(), resolved.Bucket, objectPath); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

type deleteManyReq struct {
	Prefixes []string `json:"prefixes"`
}

func (h *Handler) deleteManyObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	resolved, eng, err := h.registry.Resolve(bucketID)
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
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]model.FileObject, 0, len(req.Prefixes))
	for _, p := range req.Prefixes {
		out = append(out, model.FileObject{Name: p})
	}
	writeJSON(w, http.StatusOK, out)
}

type listReq struct {
	Prefix string `json:"prefix"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	SortBy *struct {
		Column string `json:"column"`
		Order  string `json:"order"`
	} `json:"sortBy"`
}

func (h *Handler) listObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	resolved, eng, err := h.registry.Resolve(bucketID)
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
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]model.FileObject, 0, len(objs))
	for _, o := range objs {
		out = append(out, toFileObject(resolved, o))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) listObjectsV2(w http.ResponseWriter, r *http.Request) {
	h.listObjects(w, r)
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
	src, srcEng, err := h.registry.Resolve(req.BucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	dstRef := req.DestinationBucket
	if dstRef == "" {
		dstRef = req.BucketID
	}
	dst, dstEng, err := h.registry.Resolve(dstRef)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if src.Engine != dst.Engine || srcEng != dstEng {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "cross-engine move not supported")
		return
	}
	if err := srcEng.CopyObject(r.Context(), src.Bucket, req.SourceKey, dst.Bucket, req.DestinationKey); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	_ = srcEng.DeleteObject(r.Context(), src.Bucket, req.SourceKey)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully moved"})
}

func (h *Handler) copyObject(w http.ResponseWriter, r *http.Request) {
	var req moveCopyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	src, srcEng, err := h.registry.Resolve(req.BucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	dstRef := req.DestinationBucket
	if dstRef == "" {
		dstRef = req.BucketID
	}
	dst, dstEng, err := h.registry.Resolve(dstRef)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if src.Engine != dst.Engine || srcEng != dstEng {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "cross-engine copy not supported")
		return
	}
	if err := srcEng.CopyObject(r.Context(), src.Bucket, req.SourceKey, dst.Bucket, req.DestinationKey); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"Key": req.DestinationKey})
}

type signReq struct {
	ExpiresIn int `json:"expiresIn"`
	Paths     []string `json:"paths"`
}

func (h *Handler) signObject(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	var req signReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	expires := h.cfg.PresignExpires
	if req.ExpiresIn > 0 {
		expires = time.Duration(req.ExpiresIn) * time.Second
	}

	resolved, _, err := h.registry.Resolve(bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	token, err := engine.IssueToken(h.cfg.SigningSecret, engine.SignClaims{
		Engine: resolved.Engine,
		Bucket: resolved.Bucket,
		Path:   objectPath,
		Op:     "download",
		Exp:    time.Now().Add(expires).Unix(),
	})
	if err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	signedPath := "/storage/v1/object/sign/" + resolved.DisplayID + "/" + escapeObjectPath(objectPath)
	signedURL := signedPath + "?token=" + url.QueryEscape(token)
	writeJSON(w, http.StatusOK, map[string]string{"signedURL": signedURL})
}

func (h *Handler) signManyObjects(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	var req signReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	expires := h.cfg.PresignExpires
	if req.ExpiresIn > 0 {
		expires = time.Duration(req.ExpiresIn) * time.Second
	}
	resolved, _, err := h.registry.Resolve(bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	out := make([]map[string]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		token, err := engine.IssueToken(h.cfg.SigningSecret, engine.SignClaims{
			Engine: resolved.Engine,
			Bucket: resolved.Bucket,
			Path:   p,
			Op:     "download",
			Exp:    time.Now().Add(expires).Unix(),
		})
		if err != nil {
			writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		signedPath := "/storage/v1/object/sign/" + resolved.DisplayID + "/" + escapeObjectPath(p)
		out = append(out, map[string]string{
			"path":      p,
			"signedURL": signedPath + "?token=" + url.QueryEscape(token),
			"error":     "",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getSignedObject(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}
	claims, err := engine.VerifyToken(h.cfg.SigningSecret, token)
	if err != nil {
		writeStorageErr(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if claims.Op != "download" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid token op")
		return
	}
	eng, err := h.registry.Engine(claims.Engine)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	body, info, err := eng.GetObject(r.Context(), claims.Bucket, claims.Path)
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
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handler) signUpload(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	objectPath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	resolved, _, err := h.registry.Resolve(bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	expires := h.cfg.PresignExpires
	token, err := engine.IssueToken(h.cfg.SigningSecret, engine.SignClaims{
		Engine: resolved.Engine,
		Bucket: resolved.Bucket,
		Path:   objectPath,
		Op:     "upload",
		Exp:    time.Now().Add(expires).Unix(),
	})
	if err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	path := "/storage/v1/object/upload/sign/" + resolved.DisplayID + "/" + escapeObjectPath(objectPath)
	writeJSON(w, http.StatusOK, map[string]string{"url": path + "?token=" + url.QueryEscape(token)})
}

func (h *Handler) uploadSigned(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}
	claims, err := engine.VerifyToken(h.cfg.SigningSecret, token)
	if err != nil {
		writeStorageErr(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if claims.Op != "upload" {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid token op")
		return
	}
	bucketID := engine.FormatBucketID(h.registry.DefaultEngine(), claims.Engine, claims.Bucket)
	eng, err := h.registry.Engine(claims.Engine)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	body, contentType, _, metadata, err := readUploadBody(r)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	defer body.Close()

	if !upsertHeader(r) {
		if _, err := eng.HeadObject(r.Context(), claims.Bucket, claims.Path); err == nil {
			writeStorageErr(w, http.StatusConflict, "Duplicate", "The resource already exists")
			return
		}
	}
	if err := eng.PutObject(r.Context(), claims.Bucket, claims.Path, contentType, body, metadata); err != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"Key": bucketID + "/" + claims.Path,
		"path": claims.Path,
	})
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
		case "":
			filePart = part
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

func escapeObjectPath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
