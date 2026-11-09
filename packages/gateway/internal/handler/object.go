package handler

import (
	"net/http"
	"strings"
	"time"
)

type objectListReq struct {
	Bucket            string `json:"bucket"`
	Prefix            string `json:"prefix"`
	Delimiter         string `json:"delimiter"`
	ContinuationToken string `json:"continuationToken"`
	MaxKeys           int    `json:"maxKeys"`
}

type objectDeleteReq struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type objectCopyReq struct {
	Bucket  string `json:"bucket"`
	SrcKey  string `json:"srcKey"`
	DestKey string `json:"destKey"`
}

func (h *Handler) objectList(r *http.Request, req *objectListReq) (any, error) {
	if err := requireBucket(req.Bucket); err != nil {
		return nil, err
	}
	prefix := strings.TrimPrefix(strings.TrimSpace(req.Prefix), "/")
	if prefix != "" {
		if err := validateKey(prefix); err != nil {
			return nil, err
		}
	}
	if req.MaxKeys < 0 {
		return nil, errBadRequest("maxKeys must be >= 0")
	}
	maxKeys := req.MaxKeys
	if maxKeys == 0 || maxKeys > maxListKeys {
		maxKeys = maxListKeys
	}
	// Authorize against the listed prefix (empty prefix = bucket root).
	ad, err := h.resolve(r, req.Bucket, prefix, false)
	if err != nil {
		return nil, err
	}
	res, err := ad.ListObjects(r.Context(), req.Bucket, prefix, req.Delimiter, req.ContinuationToken, maxKeys)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(res.Entries))
	for _, e := range res.Entries {
		item := map[string]any{
			"key":   e.Key,
			"isDir": e.IsDir,
		}
		if !e.IsDir {
			item["size"] = e.Size
			if e.ETag != "" {
				item["etag"] = e.ETag
			}
			if !e.LastModified.IsZero() {
				item["lastModified"] = e.LastModified.UTC().Format(time.RFC3339)
			}
			item["cdnUrl"] = ad.CDNURL(req.Bucket, e.Key)
		}
		entries = append(entries, item)
	}
	return map[string]any{
		"entries":               entries,
		"isTruncated":           res.IsTruncated,
		"nextContinuationToken": res.NextContinuationToken,
	}, nil
}

func (h *Handler) objectDelete(r *http.Request, req *objectDeleteReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.Key); err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.Key, true)
	if err != nil {
		return nil, err
	}
	if err := ad.DeleteObject(r.Context(), req.Bucket, req.Key); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func (h *Handler) objectCopy(r *http.Request, req *objectCopyReq) (any, error) {
	if err := requireBucketKey(req.Bucket, req.SrcKey); err != nil {
		return nil, err
	}
	if err := requireBucketKey(req.Bucket, req.DestKey); err != nil {
		return nil, err
	}
	if _, err := h.resolve(r, req.Bucket, req.SrcKey, false); err != nil {
		return nil, err
	}
	ad, err := h.resolve(r, req.Bucket, req.DestKey, true)
	if err != nil {
		return nil, err
	}
	if err := ad.CopyObject(r.Context(), req.Bucket, req.SrcKey, req.DestKey); err != nil {
		return nil, err
	}
	return map[string]string{
		"status": "ok",
		"cdnUrl": ad.CDNURL(req.Bucket, req.DestKey),
	}, nil
}
