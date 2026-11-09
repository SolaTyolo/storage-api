package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SolaTyolo/storage-api/internal/adapter"
	"github.com/SolaTyolo/storage-api/internal/auth"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/handler"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const shutdownTimeout = 20 * time.Second

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("gateway exited", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.AllowNoJWT {
		log.Warn("ALLOW_NO_JWT is enabled; gateway accepts unauthenticated requests (dev only)")
	}

	registry, err := adapter.NewRegistry(cfg)
	if err != nil {
		return err
	}

	authenticator := auth.New(auth.Options{
		Secret:     cfg.JWTSecret,
		AllowNoJWT: cfg.AllowNoJWT,
		Issuer:     cfg.JWTIssuer,
		Audience:   cfg.JWTAudience,
	})
	h := handler.New(cfg, registry, authenticator, log)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("gateway listening", "addr", cfg.ListenAddr, "buckets", len(cfg.Buckets), "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("gateway stopped")
	return nil
}
