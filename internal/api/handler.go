package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/SolaTyolo/storage-api/internal/authz"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

type Handler struct {
	cfg       config.Config
	registry  *engine.Registry
	transform *transform.Service
	preview   *preview.Service
	authz     authz.Authorizer
	jobs      *preview.JobStore
	log       *slog.Logger
}

func NewRouter(cfg config.Config, registry *engine.Registry, tf *transform.Service, prev *preview.Service, log *slog.Logger) http.Handler {
	var jobs *preview.JobStore
	if cfg.PreviewAsync {
		jobs = preview.NewJobStore(cfg.PreviewJobTTL)
	}
	h := &Handler{
		cfg:       cfg,
		registry:  registry,
		transform: tf,
		preview:   prev,
		authz:     authz.NewFromConfig(cfg.AuthzHTTPURL, cfg.AuthzHTTPTimeoutSec),
		jobs:      jobs,
		log:       log,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, metricsMiddleware, accessLogger(log), middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "apikey", "x-api-key", "x-upsert", "x-client-info", "cache-control", "x-metadata"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", h.health)
	r.Head("/health", h.health)
	r.Handle("/metrics", metricsHandler())

	r.Route("/storage/v1", func(r chi.Router) {
		r.Use(h.requireAPIKey)
		r.Use(h.requireAuthz)

		// Supabase bucket API
		r.Post("/bucket", h.createBucket)
		r.Get("/bucket", h.listBuckets)
		r.Head("/bucket", h.listBuckets)
		r.Get("/bucket/{bucketId}", h.getBucket)
		r.Head("/bucket/{bucketId}", h.getBucket)
		r.Put("/bucket/{bucketId}", h.updateBucket)
		r.Delete("/bucket/{bucketId}", h.deleteBucket)
		r.Post("/bucket/{bucketId}/empty", h.emptyBucket)

		// Supabase object API
		r.Post("/object/list/{bucketName}", h.listObjects)
		r.Post("/object/list-v2/{bucketName}", h.listObjectsV2)
		r.Post("/object/move", h.moveObject)
		r.Post("/object/copy", h.copyObject)
		r.Delete("/object/{bucketName}", h.deleteManyObjects)

		r.Post("/object/upload/sign/{bucketName}/*", h.signUpload)
		r.Put("/object/upload/sign/{bucketName}/*", h.uploadSigned)

		r.Post("/object/sign/{bucketName}", h.signManyObjects)
		r.Post("/object/sign/{bucketName}/*", h.signObject)
		r.Get("/object/sign/{bucketName}/*", h.getSignedObject)
		r.Head("/object/sign/{bucketName}/*", h.getSignedObject)

		r.Get("/object/info/{bucketName}/*", h.objectInfo)
		r.Head("/object/info/{bucketName}/*", h.headObject)

		r.Get("/object/public/{bucketName}/*", h.getPublicObject)
		r.Head("/object/public/{bucketName}/*", h.headPublicObject)

		r.Get("/object/authenticated/{bucketName}/*", h.getObject)
		r.Head("/object/authenticated/{bucketName}/*", h.headObject)

		r.Post("/object/{bucketName}/*", h.uploadObject)
		r.Put("/object/{bucketName}/*", h.updateObject)
		r.Get("/object/{bucketName}/*", h.getObject)
		r.Head("/object/{bucketName}/*", h.headObject)
		r.Delete("/object/{bucketName}/*", h.deleteOneObject)

		// Image transform / preview (Supabase + extended PDF/Office/video)
		r.Get("/render/image/public/{bucketName}/*", h.renderPublicImage)
		r.Head("/render/image/public/{bucketName}/*", h.renderPublicImage)
		r.Get("/render/image/authenticated/{bucketName}/*", h.renderImage)
		r.Head("/render/image/authenticated/{bucketName}/*", h.renderImage)

		r.Get("/render/job/{jobId}", h.getRenderJob)
		r.Head("/render/job/{jobId}", h.getRenderJob)
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.registry.Ping(r.Context()); err != nil {
		h.logWarn(r, "health check degraded", "error", err.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "storage": err.Error()})
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
