package handler

import (
	"fmt"
	"net/http"

	"github.com/SolaTyolo/storage-api/internal/adapter"
)

type uploadReq struct {
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	ContentType string            `json:"contentType"`
	Meta        map[string]string `json:"meta"`
	ExpireSec   int               `json:"expireSec"`
	// Size is the exact upload size in bytes. For presigned PUT it is signed as
	// Content-Length; it is required when the bucket sets maxUploadBytes.
	Size *int64 `json:"size"`
}

type multipartPresignPartReq struct {
	Bucket     string `json:"bucket"`
	Key        string `json:"key"`
	UploadID   string `json:"uploadId"`
	PartNumber int    `json:"partNumber"`
	ExpireSec  int    `json:"expireSec"`
}

type multipartPresignPartsReq struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	UploadID    string `json:"uploadId"`
	PartNumbers []int  `json:"partNumbers"`
	ExpireSec   int    `json:"expireSec"`
}

type partRef struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type signedPart struct {
	PartNumber int    `json:"partNumber"`
	URL        string `json:"url"`
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

func (h *Handler) presignPut(r *http.Request, req *uploadReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	size, err := h.checkUpload(req.Bucket, req.ContentType, req.Size)
	if err != nil {
		return nil, err
	}
	expiry, err := h.expire(req.ExpireSec)
	if err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	uploadURL, err := ad.PresignPut(r.Context(), req.Bucket, req.Key, req.ContentType, size, req.Meta, expiry)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"uploadUrl": uploadURL,
		"cdnUrl":    ad.CDNURL(req.Bucket, req.Key),
	}, nil
}

func (h *Handler) multipartCreate(r *http.Request, req *uploadReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	if _, err := h.checkUpload(req.Bucket, req.ContentType, req.Size); err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	uploadID, err := ad.CreateMultipart(r.Context(), req.Bucket, req.Key, req.ContentType, req.Meta)
	if err != nil {
		return nil, err
	}
	return map[string]string{"uploadId": uploadID}, nil
}

func validatePartNumber(n int) error {
	if n < 1 || n > maxPartNumber {
		return errBadRequest(fmt.Sprintf("partNumber must be between 1 and %d", maxPartNumber))
	}
	return nil
}

func (h *Handler) multipartPresignPart(r *http.Request, req *multipartPresignPartReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	if req.UploadID == "" {
		return nil, errBadRequest("uploadId is required")
	}
	if err := validatePartNumber(req.PartNumber); err != nil {
		return nil, err
	}
	expiry, err := h.expire(req.ExpireSec)
	if err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	partURL, err := ad.PresignUploadPart(r.Context(), req.Bucket, req.Key, req.UploadID, req.PartNumber, expiry)
	if err != nil {
		return nil, err
	}
	return map[string]string{"partUploadUrl": partURL}, nil
}

func (h *Handler) multipartPresignParts(r *http.Request, req *multipartPresignPartsReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	if req.UploadID == "" {
		return nil, errBadRequest("uploadId is required")
	}
	if len(req.PartNumbers) == 0 || len(req.PartNumbers) > maxPresignBatch {
		return nil, errBadRequest(fmt.Sprintf("partNumbers must contain 1 to %d entries", maxPresignBatch))
	}
	for _, n := range req.PartNumbers {
		if err := validatePartNumber(n); err != nil {
			return nil, err
		}
	}
	expiry, err := h.expire(req.ExpireSec)
	if err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	parts := make([]signedPart, 0, len(req.PartNumbers))
	for _, n := range req.PartNumbers {
		u, err := ad.PresignUploadPart(r.Context(), req.Bucket, req.Key, req.UploadID, n, expiry)
		if err != nil {
			return nil, err
		}
		parts = append(parts, signedPart{PartNumber: n, URL: u})
	}
	return map[string]any{"parts": parts}, nil
}

func (h *Handler) multipartComplete(r *http.Request, req *multipartCompleteReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	if req.UploadID == "" || len(req.Parts) == 0 {
		return nil, errBadRequest("uploadId and parts are required")
	}
	if len(req.Parts) > maxPartNumber {
		return nil, errBadRequest(fmt.Sprintf("at most %d parts are allowed", maxPartNumber))
	}
	parts := make([]adapter.CompletePart, 0, len(req.Parts))
	for _, p := range req.Parts {
		if err := validatePartNumber(p.PartNumber); err != nil {
			return nil, err
		}
		if p.ETag == "" {
			return nil, errBadRequest("every part requires an etag")
		}
		parts = append(parts, adapter.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	etag, err := ad.CompleteMultipart(r.Context(), req.Bucket, req.Key, req.UploadID, parts)
	if err != nil {
		return nil, err
	}
	// Part URLs cannot bind a size, so enforce the bucket limit on the assembled object.
	if bc, _ := h.cfg.GetBucket(req.Bucket); bc.MaxUploadBytes > 0 {
		info, err := ad.HeadObject(r.Context(), req.Bucket, req.Key)
		if err != nil {
			return nil, err
		}
		if info.Size > bc.MaxUploadBytes {
			if err := ad.DeleteObject(r.Context(), req.Bucket, req.Key); err != nil {
				return nil, err
			}
			return nil, errTooLarge(bc.MaxUploadBytes)
		}
	}
	return map[string]string{"etag": etag}, nil
}

func (h *Handler) multipartAbort(r *http.Request, req *multipartAbortReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	if req.UploadID == "" {
		return nil, errBadRequest("uploadId is required")
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	if err := ad.AbortMultipart(r.Context(), req.Bucket, req.Key, req.UploadID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func (h *Handler) presignGet(r *http.Request, req *presignGetReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	expiry, err := h.expire(req.ExpireSec)
	if err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, false)
	if err != nil {
		return nil, err
	}
	downloadURL, err := ad.PresignGet(r.Context(), req.Bucket, req.Key, expiry)
	if err != nil {
		return nil, err
	}
	return map[string]string{"downloadUrl": downloadURL}, nil
}

func (h *Handler) objectHead(r *http.Request, req *objectHeadReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, false)
	if err != nil {
		return nil, err
	}
	info, err := ad.HeadObject(r.Context(), req.Bucket, req.Key)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"size":        info.Size,
		"contentType": info.ContentType,
		"meta":        info.Meta,
		"etag":        info.ETag,
		"cdnUrl":      ad.CDNURL(req.Bucket, req.Key),
	}, nil
}
