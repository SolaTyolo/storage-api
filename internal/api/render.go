package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/SolaTyolo/storage-api/internal/mime"
	"github.com/SolaTyolo/storage-api/internal/model"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

func (h *Handler) renderImage(w http.ResponseWriter, r *http.Request) {
	h.renderImageInternal(w, r, false)
}

func (h *Handler) renderPublicImage(w http.ResponseWriter, r *http.Request) {
	h.renderImageInternal(w, r, true)
}

func (h *Handler) renderImageInternal(w http.ResponseWriter, r *http.Request, requirePublic bool) {
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

	info, err := eng.HeadObject(r.Context(), resolved.Bucket, objectPath)
	if err != nil {
		if engineIsNotFound(err) {
			writeStorageErr(w, http.StatusNotFound, "not_found", "object not found")
			return
		}
		writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	params, err := transform.ParseSupabaseTransform(r.URL.Query(), h.cfg.TransformMaxEdge)
	if err != nil {
		writeStorageErr(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	contentType := info.ContentType
	kind := mime.Classify(contentType)

	if h.cfg.TransformBackend == "imgproxy" && kind == mime.KindImage {
		proxyURL, err := transform.ImgproxyURL(h.cfg, resolved.Bucket, objectPath, params)
		if err != nil {
			writeStorageErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		http.Redirect(w, r, proxyURL, http.StatusFound)
		return
	}

	obj := model.ObjectRef{
		Engine:      resolved.Engine,
		Bucket:      resolved.Bucket,
		Path:        objectPath,
		ContentType: contentType,
		Size:        info.Size,
	}

	var body []byte
	var outCT string
	var renderErr error

	switch kind {
	case mime.KindPDF, mime.KindDocument:
		page, dpi := params.Page, params.DPI
		if page <= 0 {
			page = 1
		}
		if dpi <= 0 {
			dpi = 150
		}
		jpeg, rerr := h.preview.Rasterize(r.Context(), obj, page, dpi)
		if errors.Is(rerr, preview.ErrNotSupported) {
			writeStorageErr(w, http.StatusUnsupportedMediaType, "not_supported", rerr.Error())
			return
		}
		if rerr != nil {
			writeStorageErr(w, http.StatusInternalServerError, "internal", rerr.Error())
			return
		}
		body, outCT, renderErr = h.transform.RenderJPEG(jpeg, params)
	default:
		body, outCT, renderErr = h.transform.Render(r.Context(), obj, params)
	}

	if errors.Is(renderErr, transform.ErrNotSupported) {
		writeStorageErr(w, http.StatusUnsupportedMediaType, "not_supported", "delivery not supported for this media type")
		return
	}
	if renderErr != nil {
		writeStorageErr(w, http.StatusInternalServerError, "internal", renderErr.Error())
		return
	}

	w.Header().Set("Content-Type", outCT)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
