package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// S3MaxPresignExpire is the SigV4 upper bound for presigned URL lifetime.
const S3MaxPresignExpire = 7 * 24 * time.Hour

// BucketConfig describes one S3-compatible bucket endpoint.
type BucketConfig struct {
	Name string `json:"name"`
	// Endpoint is the internal S3 API URL used by the gateway (e.g. http://rustfs:9000).
	Endpoint string `json:"endpoint"`
	// PublicEndpoint is the browser-facing S3 host used when signing URLs (e.g. http://localhost:9000).
	// SigV4 includes Host — must match what the browser calls. When empty, Endpoint is used.
	PublicEndpoint string `json:"publicEndpoint,omitempty"`
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey"`
	CDNDomain      string `json:"cdnDomain"`
	UseSSL         bool   `json:"useSSL"`
	Region         string `json:"region,omitempty"`
	// Addressing selects URL style: "auto" (default), "path", or "virtual".
	// Tencent COS requires virtual-hosted URLs. Auto enables virtual for *.myqcloud.com.
	// Endpoint must be the regional host (cos.ap-shanghai.myqcloud.com), not <bucket>.cos.…
	// With virtual style, Name becomes the subdomain host — it is NOT appended to the object path.
	Addressing string `json:"addressing,omitempty"`
	// MaxUploadBytes caps object size for presigned PUT and multipart uploads (0 = unlimited).
	// When set, clients must declare the upload size so it can be signed into the URL.
	MaxUploadBytes int64 `json:"maxUploadBytes,omitempty"`
	// AllowedContentTypes restricts upload Content-Type (exact match or "type/*"). Empty allows any.
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty"`
}

// Config is gateway runtime configuration from environment.
type Config struct {
	ListenAddr  string
	JWTSecret   string
	JWTIssuer   string
	JWTAudience string
	// AllowNoJWT disables authentication. Requires ALLOW_NO_JWT=true and an empty JWT_SECRET.
	AllowNoJWT         bool
	DefaultExpire      time.Duration
	MaxExpire          time.Duration
	CORSAllowedOrigins []string
	MaxBodyBytes       int64
	Buckets            []BucketConfig
	BucketByName       map[string]BucketConfig
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	defaultExpire, err := envInt("DEFAULT_EXPIRE_SEC", 900)
	if err != nil {
		return nil, err
	}
	maxExpire, err := envInt("MAX_EXPIRE_SEC", 86400)
	if err != nil {
		return nil, err
	}
	maxBody, err := envInt("MAX_BODY_BYTES", 1<<20)
	if err != nil {
		return nil, err
	}
	allowNoJWT, err := envBool("ALLOW_NO_JWT")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ListenAddr:         envOr("LISTEN_ADDR", ":8080"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		JWTIssuer:          strings.TrimSpace(os.Getenv("JWT_ISSUER")),
		JWTAudience:        strings.TrimSpace(os.Getenv("JWT_AUDIENCE")),
		DefaultExpire:      time.Duration(defaultExpire) * time.Second,
		MaxExpire:          time.Duration(maxExpire) * time.Second,
		CORSAllowedOrigins: splitList(envOr("CORS_ALLOWED_ORIGINS", "*")),
		MaxBodyBytes:       int64(maxBody),
		BucketByName:       make(map[string]BucketConfig),
	}

	switch {
	case cfg.JWTSecret != "":
		cfg.AllowNoJWT = false
	case allowNoJWT:
		cfg.AllowNoJWT = true
	default:
		return nil, fmt.Errorf("JWT_SECRET is required (set ALLOW_NO_JWT=true to disable auth for local dev)")
	}

	if cfg.MaxExpire <= 0 || cfg.MaxExpire > S3MaxPresignExpire {
		return nil, fmt.Errorf("MAX_EXPIRE_SEC must be between 1 and %d", int(S3MaxPresignExpire.Seconds()))
	}
	if cfg.DefaultExpire <= 0 || cfg.DefaultExpire > cfg.MaxExpire {
		return nil, fmt.Errorf("DEFAULT_EXPIRE_SEC must be between 1 and MAX_EXPIRE_SEC")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, fmt.Errorf("MAX_BODY_BYTES must be positive")
	}
	if len(cfg.CORSAllowedOrigins) == 0 {
		return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS must not be empty")
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
		if _, dup := cfg.BucketByName[b.Name]; dup {
			return nil, fmt.Errorf("bucket %q is configured more than once", b.Name)
		}
		if b.Endpoint == "" || b.AccessKey == "" || b.SecretKey == "" {
			return nil, fmt.Errorf("bucket %q: endpoint, accessKey, secretKey are required", b.Name)
		}
		if err := validateEndpoint(b.Endpoint); err != nil {
			return nil, fmt.Errorf("bucket %q: endpoint: %w", b.Name, err)
		}
		if b.PublicEndpoint != "" {
			if err := validateEndpoint(b.PublicEndpoint); err != nil {
				return nil, fmt.Errorf("bucket %q: publicEndpoint: %w", b.Name, err)
			}
		}
		if b.MaxUploadBytes < 0 {
			return nil, fmt.Errorf("bucket %q: maxUploadBytes must be >= 0", b.Name)
		}
		if err := validateAddressing(b.Addressing); err != nil {
			return nil, fmt.Errorf("bucket %q: %w", b.Name, err)
		}
		if err := rejectCOSBucketHost(b.Endpoint); err != nil {
			return nil, fmt.Errorf("bucket %q: endpoint: %w", b.Name, err)
		}
		if b.PublicEndpoint != "" {
			if err := rejectCOSBucketHost(b.PublicEndpoint); err != nil {
				return nil, fmt.Errorf("bucket %q: publicEndpoint: %w", b.Name, err)
			}
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

// UseVirtualHost reports whether requests should use bucket.endpoint/key style.
// Name is placed in the hostname, never as a path prefix of the object key.
func (b BucketConfig) UseVirtualHost() bool {
	switch strings.ToLower(strings.TrimSpace(b.Addressing)) {
	case "path":
		return false
	case "virtual", "dns", "virtual-hosted":
		return true
	default:
		return isTencentCOSEndpoint(b.Endpoint) || isTencentCOSEndpoint(b.PublicEndpoint)
	}
}

// ContentTypeAllowed reports whether contentType matches the bucket allowlist.
func (b BucketConfig) ContentTypeAllowed(contentType string) bool {
	if len(b.AllowedContentTypes) == 0 {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	for _, allowed := range b.AllowedContentTypes {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == ct || allowed == "*/*" {
			return true
		}
		if base, ok := strings.CutSuffix(allowed, "/*"); ok && strings.HasPrefix(ct, base+"/") {
			return true
		}
	}
	return false
}

// validateEndpoint rejects endpoints minio-go cannot use (paths, queries, bad schemes).
func validateEndpoint(raw string) error {
	s := raw
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}
	if strings.Trim(u.Path, "/") != "" || u.RawQuery != "" {
		return fmt.Errorf("must not contain a path or query (got %q)", raw)
	}
	return nil
}

func validateAddressing(v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto", "path", "virtual", "dns", "virtual-hosted":
		return nil
	default:
		return fmt.Errorf("addressing must be auto, path, or virtual (got %q)", v)
	}
}

// rejectCOSBucketHost rejects endpoints like <bucket>.cos.<region>.myqcloud.com.
// Those already include the bucket; combining them with Name duplicates the bucket in the key path.
func rejectCOSBucketHost(raw string) error {
	host := strings.ToLower(endpointHost(raw))
	if host == "" || !isTencentCOSEndpoint(raw) {
		return nil
	}
	// Regional OK: cos.ap-shanghai.myqcloud.com
	// Bucket virtual host NOT OK: develop-xxx.cos.ap-shanghai.myqcloud.com
	if strings.HasPrefix(host, "cos.") {
		return nil
	}
	if strings.Contains(host, ".cos.") {
		return fmt.Errorf("use the regional COS endpoint (e.g. https://cos.ap-shanghai.myqcloud.com), not the bucket virtual host %q", host)
	}
	return nil
}

func isTencentCOSEndpoint(raw string) bool {
	host := strings.ToLower(endpointHost(raw))
	return strings.Contains(host, ".myqcloud.com") || strings.Contains(host, ".tencentcos.cn")
}

func endpointHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return u.Host
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", key, v)
	}
	return n, nil
}

func envBool(key string) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q", key, v)
	}
	return b, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
