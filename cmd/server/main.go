package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SolaTyolo/storage-api/internal/api"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

func main() {
	cfg, storageYAML, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	registry, err := engine.LoadRegistry(storageYAML)
	if err != nil {
		log.Fatalf("engines: %v", err)
	}

	tf := transform.New(cfg, registry)
	prev := preview.New(registry, cfg.GotenbergURL, cfg.PopplerWorkerURL)
	router := api.NewRouter(cfg, registry, tf, prev)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	go func() {
		log.Printf("storage api listening on %s", cfg.HTTPAddr)
		log.Printf("default engine: %s", registry.DefaultEngine())
		log.Printf("playground: http://%s/playground/", trimLeadingColon(cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func trimLeadingColon(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "localhost" + addr
	}
	return addr
}
