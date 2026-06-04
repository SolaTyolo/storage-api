package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr string

	DatabaseURL string

	S3Endpoint        string
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Bucket          string
	S3UsePathStyle    bool

	PresignExpires       time.Duration
	UploadSigningSecret  string

	TransformBackend   string // internal | imgproxy
	TransformMaxEdge   int
	ImgproxyBaseURL    string
	ImgproxyInsecure   bool

	GotenbergURL     string
	PopplerWorkerURL string
	FFmpegPath       string
}

func Load() Config {
	return Config{
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),

		DatabaseURL: getenv("DATABASE_URL", "postgres://storage:storage@localhost:5432/storage?sslmode=disable"),

		S3Endpoint:        getenv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:          getenv("S3_REGION", "us-east-1"),
		S3AccessKeyID:     getenv("S3_ACCESS_KEY_ID", "rustfsadmin"),
		S3SecretAccessKey: getenv("S3_SECRET_ACCESS_KEY", "rustfsadmin"),
		S3Bucket:          getenv("S3_BUCKET", "uploads"),
		S3UsePathStyle:    getenvBool("S3_USE_PATH_STYLE", true),

		PresignExpires:      time.Duration(getenvInt("PRESIGN_EXPIRES_SEC", 3600)) * time.Second,
		UploadSigningSecret: getenv("UPLOAD_SIGNING_SECRET", "dev-upload-secret-change-me"),
		TransformBackend:    getenv("TRANSFORM_BACKEND", "internal"),
		TransformMaxEdge:    getenvInt("TRANSFORM_MAX_EDGE", 4096),
		ImgproxyBaseURL:     getenv("IMGPROXY_BASE_URL", "http://localhost:8081"),
		ImgproxyInsecure:    getenvBool("IMGPROXY_INSECURE", true),
		GotenbergURL:        getenv("GOTENBERG_URL", "http://localhost:3000"),
		PopplerWorkerURL:    getenv("POPPLER_WORKER_URL", "http://localhost:8090"),
		FFmpegPath:          getenv("FFMPEG_PATH", "ffmpeg"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		return v == "1" || v == "true" || v == "yes"
	}
	return def
}
