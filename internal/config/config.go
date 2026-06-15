package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr string

	StorageConfigPath string
	DefaultEngine     string

	SigningSecret  string
	PresignExpires time.Duration

	TransformBackend string // internal | imgproxy
	TransformMaxEdge int
	ImgproxyBaseURL  string
	ImgproxyInsecure bool

	GotenbergURL     string
	PopplerWorkerURL string
	FFmpegPath       string
}

type StorageYAML struct {
	DefaultEngine string                 `yaml:"default_engine"`
	Engines       map[string]EngineYAML  `yaml:"engines"`
}

type EngineYAML struct {
	Type            string `yaml:"type"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	PathStyle       bool   `yaml:"path_style"`
}

func Load() (Config, StorageYAML, error) {
	cfg := Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		StorageConfigPath: getenv("STORAGE_CONFIG_PATH", "config/storage.yaml"),
		DefaultEngine:     os.Getenv("STORAGE_DEFAULT_ENGINE"),
		SigningSecret:     getenv("UPLOAD_SIGNING_SECRET", "dev-upload-secret-change-me"),
		PresignExpires:    time.Duration(getenvInt("PRESIGN_EXPIRES_SEC", 3600)) * time.Second,
		TransformBackend:  getenv("TRANSFORM_BACKEND", "internal"),
		TransformMaxEdge:  getenvInt("TRANSFORM_MAX_EDGE", 4096),
		ImgproxyBaseURL:   getenv("IMGPROXY_BASE_URL", "http://localhost:8081"),
		ImgproxyInsecure:  getenvBool("IMGPROXY_INSECURE", true),
		GotenbergURL:      getenv("GOTENBERG_URL", "http://localhost:3000"),
		PopplerWorkerURL:  getenv("POPPLER_WORKER_URL", "http://localhost:8090"),
		FFmpegPath:        getenv("FFMPEG_PATH", "ffmpeg"),
	}

	data, err := os.ReadFile(cfg.StorageConfigPath)
	if err != nil {
		return cfg, StorageYAML{}, fmt.Errorf("read storage config %s: %w", cfg.StorageConfigPath, err)
	}
	var storage StorageYAML
	if err := yaml.Unmarshal(data, &storage); err != nil {
		return cfg, StorageYAML{}, fmt.Errorf("parse storage config: %w", err)
	}
	if cfg.DefaultEngine != "" {
		storage.DefaultEngine = cfg.DefaultEngine
	}
	if storage.DefaultEngine == "" {
		return cfg, StorageYAML{}, fmt.Errorf("default_engine is required in storage config or STORAGE_DEFAULT_ENGINE env")
	}
	return cfg, storage, nil
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
