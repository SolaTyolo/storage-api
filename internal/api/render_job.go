package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/model"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

func (h *Handler) getRenderJob(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeStorageErr(w, http.StatusNotFound, "not_found", "async preview is disabled")
		return
	}
	jobID := chi.URLParam(r, "jobId")
	job, ok := h.jobs.Get(jobID)
	if !ok {
		writeStorageErr(w, http.StatusNotFound, "not_found", "render job not found")
		return
	}
	switch job.Status {
	case preview.JobPending:
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": string(preview.JobPending),
			"job_id": job.ID,
		})
	case preview.JobFailed:
		writeStorageErr(w, http.StatusInternalServerError, "internal", job.Err)
	case preview.JobReady:
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", job.ContentType)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", job.ContentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(job.Body)
	}
}

func (h *Handler) startAsyncRasterize(w http.ResponseWriter, r *http.Request, obj model.ObjectRef, page, dpi int, params transform.Params) {
	job := h.jobs.Create()
	w.Header().Set("Location", "/storage/v1/render/job/"+job.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": string(preview.JobPending),
		"job_id": job.ID,
	})

	go func() {
		ctx := context.Background()
		jpeg, err := h.preview.Rasterize(ctx, obj, page, dpi)
		if err != nil {
			h.jobs.Fail(job.ID, err.Error())
			return
		}
		body, outCT, err := h.transform.RenderJPEG(jpeg, params)
		if err != nil {
			h.jobs.Fail(job.ID, err.Error())
			return
		}
		h.jobs.Complete(job.ID, outCT, body)
	}()
}
