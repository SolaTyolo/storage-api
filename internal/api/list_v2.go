package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/model"
)

type sortBySpec struct {
	Column string `json:"column"`
	Order  string `json:"order"`
}

type listV2Req struct {
	Prefix        string `json:"prefix"`
	Limit         int    `json:"limit"`
	Cursor        string `json:"cursor"`
	WithDelimiter bool   `json:"with_delimiter"`
	SortBy        *sortBySpec `json:"sortBy"`
}

func (h *Handler) listObjectsV2(w http.ResponseWriter, r *http.Request) {
	bucketID := chi.URLParam(r, "bucketName")
	resolved, eng, err := h.registry.Resolve(r.Context(), bucketID)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var req listV2Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}

	page, err := eng.ListObjectsV2(r.Context(), resolved.Bucket, req.Prefix, req.Limit, req.Cursor, req.WithDelimiter)
	if err != nil {
		h.logError(r, "object list-v2 failed", "engine", resolved.Engine, "bucket", resolved.Bucket, "error", err.Error())
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	objects := make([]model.FileObject, 0, len(page.Objects))
	for _, o := range page.Objects {
		objects = append(objects, toFileObject(resolved, o))
	}
	sortFileObjects(objects, req.SortBy)

	folders := make([]model.SearchV2Folder, 0, len(page.Folders))
	for _, f := range page.Folders {
		key := f
		if len(key) > 0 && key[len(key)-1] == '/' {
			key = key[:len(key)-1]
		}
		folders = append(folders, model.SearchV2Folder{Name: f, Key: key})
	}

	h.logOp(r, slog.LevelDebug, "objects list-v2", "engine", resolved.Engine, "bucket", resolved.Bucket, "prefix", req.Prefix, "count", len(objects), "has_next", page.HasMore)

	writeJSON(w, http.StatusOK, model.SearchV2Result{
		HasNext:    page.HasMore,
		Folders:    folders,
		Objects:    objects,
		NextCursor: page.NextCursor,
	})
}

func sortFileObjects(objs []model.FileObject, sortBy *sortBySpec) {
	if sortBy == nil || len(objs) < 2 {
		return
	}
	desc := sortBy.Order == "desc"
	col := sortBy.Column
	if col == "" {
		col = "name"
	}

	sort.Slice(objs, func(i, j int) bool {
		switch col {
		case "updated_at", "created_at":
			if desc {
				return objs[i].UpdatedAt.After(objs[j].UpdatedAt)
			}
			return objs[i].UpdatedAt.Before(objs[j].UpdatedAt)
		default:
			if desc {
				return objs[i].Name > objs[j].Name
			}
			return objs[i].Name < objs[j].Name
		}
	})
}
