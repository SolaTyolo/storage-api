package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// BucketConfig describes one S3-compatible bucket endpoint.
type BucketConfig struct {
	Name string `json:"name"`
	// Endpoint is the internal S3 API URL used by the gateway (e.g. http://rustfs:9000).
	Endpoint string `json:"endpoint"`
	// PublicEndpoint rewrites hosts in presigned URLs for browser clients (e.g. http://localhost:9000).
	// When empty, Endpoint is used as-is.
	PublicEndpoint string `json:"publicEndpoint,omitempty"`
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey"`
	CDNDomain      string `json:"cdnDomain"`
	UseSSL         bool   `json:"useSSL"`
	Region         string `json:"region,omitempty"`
}

// Config is gateway runtime configuration from environment.
type Config struct {
	ListenAddr     string
	JWTSecret      string
	DefaultExpire  time.Duration
	Buckets        []BucketConfig
	BucketByName   map[string]BucketConfig
	AllowNoJWT     bool // when JWT_SECRET empty, allow all (dev only)
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:    envOr("LISTEN_ADDR", ":8080"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		DefaultExpire: time.Duration(envInt("DEFAULT_EXPIRE_SEC", 900)) * time.Second,
		AllowNoJWT:    os.Getenv("JWT_SECRET") == "",
		BucketByName:  make(map[string]BucketConfig),
	}

	raw := strings.TrimSpace(os.Getenv("BUCKET_CONFIG"))
	if raw == "" {
		return nil, fmt.Errorf("BUCKET_CONFIG is required")
	}
	if err := json.Unmarshal([]byte(raw), &cfg.Buckets); err != nil {
		return nil, fmt.Errorf("parse BUCKET_CONFIG: %w", err)
	}
	if len(cfg.Buckets) == 0 {
		return nil, fmt.Errorf("BUCKET_CONFIG must contain at least one bucket")
	}
	for i := range cfg.Buckets {
		b := &cfg.Buckets[i]
		if b.Name == "" {
			return nil, fmt.Errorf("bucket name is required")
		}
		if b.Endpoint == "" || b.AccessKey == "" || b.SecretKey == "" {
			return nil, fmt.Errorf("bucket %q: endpoint, accessKey, secretKey are required", b.Name)
		}
		if b.Region == "" {
			b.Region = "us-east-1"
		}
		cfg.BucketByName[b.Name] = *b
	}
	return cfg, nil
}

// GetBucket returns bucket config by name.
func (c *Config) GetBucket(name string) (BucketConfig, bool) {
	b, ok := c.BucketByName[name]
	return b, ok
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
