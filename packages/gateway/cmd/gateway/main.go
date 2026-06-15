package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/SolaTyolo/storage-api/internal/adapter"
	"github.com/SolaTyolo/storage-api/internal/auth"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/handler"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}
	if cfg.AllowNoJWT {
		log.Warn("JWT_SECRET is empty; gateway accepts unauthenticated requests (dev only)")
	}

	registry, err := adapter.NewRegistry(cfg)
	if err != nil {
		log.Error("init adapters", "error", err)
		os.Exit(1)
	}

	authenticator := auth.New(cfg.JWTSecret, cfg.AllowNoJWT)
	h := handler.New(cfg, registry, authenticator, log)

	log.Info("gateway listening", "addr", cfg.ListenAddr, "buckets", len(cfg.Buckets))
	if err := http.ListenAndServe(cfg.ListenAddr, h.Router()); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
