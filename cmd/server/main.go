package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SolaTyolo/storage-api/internal/api"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/logger"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

func main() {
	cfg, storageYAML, err := config.Load()
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		log.Error("invalid config", "error", err)
		os.Exit(1)
	}

	registry, err := engine.LoadRegistry(storageYAML)
	if err != nil {
		log.Error("failed to load engines", "error", err)
		os.Exit(1)
	}

	tf := transform.New(cfg, registry)
	prev := preview.New(registry, cfg.GotenbergURL, cfg.PopplerWorkerURL, cfg.SidecarAPIToken)
	router := api.NewRouter(cfg, registry, tf, prev, log)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	go func() {
		log.Info("storage api starting",
			"addr", cfg.HTTPAddr,
			"default_engine", registry.DefaultEngine(),
			"engines", registry.EngineNames(),
			"transform_backend", cfg.TransformBackend,
			"log_level", cfg.LogLevel,
			"log_format", cfg.LogFormat,
		)
		if len(cfg.APIKeys()) == 0 {
			log.Warn("API_KEY/API_KEYS not set; /storage/v1 is unauthenticated")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown failed", "error", err)
	}
	log.Info("storage api stopped")
}
