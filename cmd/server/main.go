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
	"github.com/SolaTyolo/storage-api/internal/s3client"
	"github.com/SolaTyolo/storage-api/internal/store"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()

	s3, err := s3client.New(cfg)
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	tf := transform.New(cfg, s3)
	prev := preview.New(s3, cfg.GotenbergURL, cfg.PopplerWorkerURL)
	router := api.NewRouter(cfg, st, s3, tf, prev)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	go func() {
		log.Printf("storage api listening on %s", cfg.HTTPAddr)
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
