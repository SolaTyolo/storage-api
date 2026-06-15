package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr string

	StorageConfigPath string
	DefaultEngine     string

	PresignExpires       time.Duration
	PublicPresignExpires time.Duration
	APIKey               string
	JWTSecret            string
	AllowPresignedUpload bool
	AuthDownloadMode     string // proxy | redirect

	AuthzHTTPURL            string
	AuthzHTTPTimeoutSec     int
	AuthzBypassServiceRole  bool
	AuthzBypassAPIKey       bool

	PreviewAsync   bool
	PreviewJobTTL  time.Duration
	SidecarAPIToken string

	TransformBackend string // internal | imgproxy
	TransformMaxEdge int
	ImgproxyBaseURL  string
	ImgproxyInsecure bool
	ImgproxyKey      string
	ImgproxySalt     string

	GotenbergURL     string
	PopplerWorkerURL string
	FFmpegPath       string

	LogLevel  string
	LogFormat string
	Env       string
}

// APIKeys returns configured API keys (API_KEYS comma-separated or single API_KEY).
func (c Config) APIKeys() []string {
	if v := os.Getenv("API_KEYS"); v != "" {
		var keys []string
		for _, k := range strings.Split(v, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			return keys
		}
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// Validate checks required production settings.
func (c Config) Validate() error {
	if strings.EqualFold(c.Env, "production") {
		if len(c.APIKeys()) == 0 && c.JWTSecret == "" {
			return fmt.Errorf("API_KEY, API_KEYS, or JWT_SECRET is required when ENV=production")
		}
		if strings.EqualFold(c.TransformBackend, "imgproxy") && !c.ImgproxyInsecure {
			if c.ImgproxyKey == "" || c.ImgproxySalt == "" {
				return fmt.Errorf("IMGPROXY_KEY and IMGPROXY_SALT are required when IMGPROXY_INSECURE=false in production")
			}
		}
	}
	mode := strings.ToLower(strings.TrimSpace(c.AuthDownloadMode))
	if mode == "" {
		mode = "proxy"
	}
	if mode != "proxy" && mode != "redirect" {
		return fmt.Errorf("AUTH_DOWNLOAD_MODE must be proxy or redirect")
	}
	return nil
}

type StorageYAML struct {
	DefaultEngine string                `yaml:"default_engine"`
	Engines       map[string]EngineYAML `yaml:"engines"`
}

type EngineYAML struct {
	Type            string `yaml:"type"`
	Endpoint        string `yaml:"endpoint"`
	PublicEndpoint  string `yaml:"public_endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	PathStyle       bool   `yaml:"path_style"`
}

func Load() (Config, StorageYAML, error) {
	cfg := Config{
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		StorageConfigPath:    getenv("STORAGE_CONFIG_PATH", "config/storage.yaml"),
		DefaultEngine:        os.Getenv("STORAGE_DEFAULT_ENGINE"),
		PresignExpires:       time.Duration(getenvInt("PRESIGN_EXPIRES_SEC", 3600)) * time.Second,
		PublicPresignExpires: time.Duration(getenvInt("PUBLIC_PRESIGN_EXPIRES_SEC", 900)) * time.Second,
		APIKey:               os.Getenv("API_KEY"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		AllowPresignedUpload: getenvBool("ALLOW_PRESIGNED_UPLOAD", true),
		AuthDownloadMode:     getenv("AUTH_DOWNLOAD_MODE", "proxy"),
		AuthzHTTPURL:            os.Getenv("AUTHZ_HTTP_URL"),
		AuthzHTTPTimeoutSec:     getenvInt("AUTHZ_HTTP_TIMEOUT_SEC", 5),
		AuthzBypassServiceRole:  getenvBool("AUTHZ_BYPASS_SERVICE_ROLE", true),
		AuthzBypassAPIKey:       getenvBool("AUTHZ_BYPASS_API_KEY", true),
		PreviewAsync:            getenvBool("PREVIEW_ASYNC", false),
		PreviewJobTTL:           time.Duration(getenvInt("PREVIEW_JOB_TTL_SEC", 900)) * time.Second,
		SidecarAPIToken:         os.Getenv("SIDECAR_API_TOKEN"),
		TransformBackend:     getenv("TRANSFORM_BACKEND", "internal"),
		TransformMaxEdge:     getenvInt("TRANSFORM_MAX_EDGE", 4096),
		ImgproxyBaseURL:        getenv("IMGPROXY_BASE_URL", "http://localhost:8081"),
		ImgproxyInsecure:       getenvBool("IMGPROXY_INSECURE", true),
		ImgproxyKey:            os.Getenv("IMGPROXY_KEY"),
		ImgproxySalt:           os.Getenv("IMGPROXY_SALT"),
		GotenbergURL:           getenv("GOTENBERG_URL", "http://localhost:3000"),
		PopplerWorkerURL:     getenv("POPPLER_WORKER_URL", "http://localhost:8090"),
		FFmpegPath:             getenv("FFMPEG_PATH", "ffmpeg"),
		LogLevel:               getenv("LOG_LEVEL", "info"),
		LogFormat:              getenv("LOG_FORMAT", "text"),
		Env:                    os.Getenv("ENV"),
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
