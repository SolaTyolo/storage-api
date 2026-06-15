package handler

import (
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"

	"github.com/SolaTyolo/storage-api/internal/adapter"
)

type presignPutReq struct {
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	ContentType string            `json:"contentType"`
	Meta        map[string]string `json:"meta"`
	ExpireSec   int               `json:"expireSec"`
}

type multipartCreateReq struct {
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	ContentType string            `json:"contentType"`
	Meta        map[string]string `json:"meta"`
	ExpireSec   int               `json:"expireSec"`
}

type multipartPresignPartReq struct {
	Bucket     string `json:"bucket"`
	Key        string `json:"key"`
	UploadID   string `json:"uploadId"`
	PartNumber int    `json:"partNumber"`
	ExpireSec  int    `json:"expireSec"`
}

type partRef struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type multipartCompleteReq struct {
	Bucket   string    `json:"bucket"`
	Key      string    `json:"key"`
	UploadID string    `json:"uploadId"`
	Parts    []partRef `json:"parts"`
}

type multipartAbortReq struct {
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	UploadID string `json:"uploadId"`
}

type presignGetReq struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	ExpireSec int    `json:"expireSec"`
}

type objectHeadReq struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

func (h *Handler) presignPut(w http.ResponseWriter, r *http.Request) {
	var req presignPutReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	uploadURL, err := ad.PresignPut(r.Context(), req.Bucket, req.Key, req.ContentType, req.Meta, h.expire(req.ExpireSec))
	if err != nil {
		h.log.Error("presign put failed", "error", err, "bucket", req.Bucket, "key", req.Key)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"uploadUrl": uploadURL,
		"cdnUrl":    ad.CDNURL(req.Bucket, req.Key),
	})
}

func (h *Handler) multipartCreate(w http.ResponseWriter, r *http.Request) {
	var req multipartCreateReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	uploadID, err := ad.CreateMultipart(r.Context(), req.Bucket, req.Key, req.ContentType, req.Meta)
	if err != nil {
		h.log.Error("multipart create failed", "error", err, "bucket", req.Bucket, "key", req.Key)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"uploadId": uploadID})
}

func (h *Handler) multipartPresignPart(w http.ResponseWriter, r *http.Request) {
	var req multipartPresignPartReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	if req.UploadID == "" || req.PartNumber < 1 {
		writeErr(w, errBadRequest("uploadId and partNumber are required"))
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	partURL, err := ad.PresignUploadPart(r.Context(), req.Bucket, req.Key, req.UploadID, req.PartNumber, h.expire(req.ExpireSec))
	if err != nil {
		h.log.Error("presign part failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"partUploadUrl": partURL})
}

func (h *Handler) multipartComplete(w http.ResponseWriter, r *http.Request) {
	var req multipartCompleteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	if req.UploadID == "" || len(req.Parts) == 0 {
		writeErr(w, errBadRequest("uploadId and parts are required"))
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	parts := make([]adapter.CompletePart, 0, len(req.Parts))
	for _, p := range req.Parts {
		parts = append(parts, adapter.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	etag, err := ad.CompleteMultipart(r.Context(), req.Bucket, req.Key, req.UploadID, parts)
	if err != nil {
		h.log.Error("multipart complete failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"etag": etag})
}

func (h *Handler) multipartAbort(w http.ResponseWriter, r *http.Request) {
	var req multipartAbortReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	if req.UploadID == "" {
		writeErr(w, errBadRequest("uploadId is required"))
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ad.AbortMultipart(r.Context(), req.Bucket, req.Key, req.UploadID); err != nil {
		h.log.Error("multipart abort failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) presignGet(w http.ResponseWriter, r *http.Request) {
	var req presignGetReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, false, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	downloadURL, err := ad.PresignGet(r.Context(), req.Bucket, req.Key, h.expire(req.ExpireSec))
	if err != nil {
		h.log.Error("presign get failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"downloadUrl": downloadURL})
}

func (h *Handler) objectHead(w http.ResponseWriter, r *http.Request) {
	var req objectHeadReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		writeErr(w, err)
		return
	}
	ad, err := h.resolve(req.Bucket, req.Key, false, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	info, err := ad.HeadObject(r.Context(), req.Bucket, req.Key)
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || errResp.StatusCode == http.StatusNotFound {
			writeErr(w, errNotFound("object not found"))
			return
		}
		h.log.Error("object head failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"size":        info.Size,
		"contentType": info.ContentType,
		"meta":        info.Meta,
		"etag":        info.ETag,
		"cdnUrl":      ad.CDNURL(req.Bucket, req.Key),
	})
}

func requireBucketKey(bucket, key string) error {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return errBadRequest("bucket and key are required")
	}
	return nil
}
