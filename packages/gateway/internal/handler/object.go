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

func (h *Handler) objectList(w http.ResponseWriter, r *http.Request) {
	var req objectListReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(req.Bucket) == "" {
		writeErr(w, errBadRequest("bucket is required"))
		return
	}
	prefix := strings.TrimPrefix(strings.TrimSpace(req.Prefix), "/")
	// Authorize against the listed prefix (empty prefix = bucket root).
	authKey := prefix
	if authKey == "" {
		authKey = "/"
	}
	ad, err := h.resolve(req.Bucket, authKey, false, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := ad.ListObjects(r.Context(), req.Bucket, prefix, req.Delimiter, req.ContinuationToken, req.MaxKeys)
	if err != nil {
		h.log.Error("object list failed", "error", err)
		writeErr(w, err)
		return
	}
	entries := make([]map[string]any, 0, len(res.Entries))
	for _, e := range res.Entries {
		item := map[string]any{
			"key":   e.Key,
			"isDir": e.IsDir,
		}
		if !e.IsDir {
			item["size"] = e.Size
			if !e.LastModified.IsZero() {
				item["lastModified"] = e.LastModified.UTC().Format(time.RFC3339)
			}
			item["cdnUrl"] = ad.CDNURL(req.Bucket, e.Key)
		}
		entries = append(entries, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries":               entries,
		"isTruncated":           res.IsTruncated,
		"nextContinuationToken": res.NextContinuationToken,
	})
}

func (h *Handler) objectDelete(w http.ResponseWriter, r *http.Request) {
	var req objectDeleteReq
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
	if err := ad.DeleteObject(r.Context(), req.Bucket, req.Key); err != nil {
		h.log.Error("object delete failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) objectCopy(w http.ResponseWriter, r *http.Request) {
	var req objectCopyReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.SrcKey); err != nil {
		writeErr(w, err)
		return
	}
	if err := requireBucketKey(req.Bucket, req.DestKey); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.resolve(req.Bucket, req.SrcKey, false, r); err != nil {
		writeErr(w, err)
		return
	}
	ad, err := h.resolve(req.Bucket, req.DestKey, true, r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ad.CopyObject(r.Context(), req.Bucket, req.SrcKey, req.DestKey); err != nil {
		h.log.Error("object copy failed", "error", err)
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"cdnUrl": ad.CDNURL(req.Bucket, req.DestKey),
	})
}
