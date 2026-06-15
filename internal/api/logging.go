package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func accessLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			// Skip noisy health probes unless unhealthy.
			if r.URL.Path == "/health" && ww.Status() < 400 {
				return
			}

			attrs := []any{
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
			}
			if ww.Status() >= 500 {
				log.Error("http request", attrs...)
			} else if ww.Status() >= 400 {
				log.Warn("http request", attrs...)
			} else {
				log.Info("http request", attrs...)
			}
		})
	}
}

func (h *Handler) logOp(r *http.Request, level slog.Level, msg string, attrs ...any) {
	if h.log == nil {
		return
	}
	base := []any{
		"request_id", middleware.GetReqID(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
	}
	h.log.Log(r.Context(), level, msg, append(base, attrs...)...)
}

func (h *Handler) logInfo(r *http.Request, msg string, attrs ...any) {
	h.logOp(r, slog.LevelInfo, msg, attrs...)
}

func (h *Handler) logWarn(r *http.Request, msg string, attrs ...any) {
	h.logOp(r, slog.LevelWarn, msg, attrs...)
}

func (h *Handler) logError(r *http.Request, msg string, attrs ...any) {
	h.logOp(r, slog.LevelError, msg, attrs...)
}

func (h *Handler) logStorage(r *http.Request, op, engine, bucket, objectPath string, attrs ...any) {
	extra := []any{"op", op, "engine", engine, "bucket", bucket, "path", objectPath}
	h.logInfo(r, "storage", append(extra, attrs...)...)
}
