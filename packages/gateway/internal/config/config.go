package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// S3MaxPresignExpire is the SigV4 upper bound for presigned URL lifetime.
const S3MaxPresignExpire = 7 * 24 * time.Hour

// Bucket env vars use BUCKET_<KEY>_<FIELD>, analogous to YAML:
//
//	bucket:
//	  uploads:
//	    endpoint: ...
//	    accessKey: ...
//
// Example: BUCKET_UPLOADS_ENDPOINT, BUCKET_UPLOADS_ACCESSKEY.
// NAME defaults to lowercased KEY when omitted.
//
// Known fields (longest match wins): NAME, ENDPOINT, PUBLICENDPOINT, ACCESSKEY,
// SECRETKEY, CDNDOMAIN, USESSL, REGION, ADDRESSING, MAXUPLOADBYTES, ALLOWEDCONTENTTYPES.
var bucketEnvFields = []string{
	"ALLOWEDCONTENTTYPES",
	"MAXUPLOADBYTES",
	"PUBLICENDPOINT",
	"CDNDOMAIN",
	"ACCESSKEY",
	"SECRETKEY",
	"ADDRESSING",
	"ENDPOINT",
	"USESSL",
	"REGION",
	"NAME",
}

// BucketConfig describes one S3-compatible bucket endpoint.
type BucketConfig struct {
	Name string
	// Endpoint is the internal S3 API URL used by the gateway (e.g. http://rustfs:9000).
	Endpoint string
	// PublicEndpoint is the browser-facing S3 host used when signing URLs (e.g. http://localhost:9000).
	// SigV4 includes Host — must match what the browser calls. When empty, Endpoint is used.
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	CDNDomain      string
	UseSSL         bool
	Region         string
	// Addressing selects URL style: "auto" (default), "path", or "virtual".
	// Tencent COS requires virtual-hosted URLs. Auto enables virtual for *.myqcloud.com.
	// Endpoint must be the regional host (cos.ap-shanghai.myqcloud.com), not <bucket>.cos.…
	// With virtual style, Name becomes the subdomain host — it is NOT appended to the object path.
	Addressing string
	// MaxUploadBytes caps object size for presigned PUT and multipart uploads (0 = unlimited).
	// When set, clients must declare the upload size so it can be signed into the URL.
	MaxUploadBytes int64
	// AllowedContentTypes restricts upload Content-Type (exact match or "type/*"). Empty allows any.
	AllowedContentTypes []string
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

	buckets, err := loadBucketsFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.Buckets = buckets
	for i := range cfg.Buckets {
		b := &cfg.Buckets[i]
		if _, dup := cfg.BucketByName[b.Name]; dup {
			return nil, fmt.Errorf("bucket %q is configured more than once", b.Name)
		}
		cfg.BucketByName[b.Name] = *b
	}
	return cfg, nil
}

// loadBucketsFromEnv discovers BUCKET_<KEY>_* groups and builds BucketConfig entries.
func loadBucketsFromEnv() ([]BucketConfig, error) {
	groups := map[string]map[string]string{}
	for _, e := range os.Environ() {
		envKey, value, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		bucketKey, field, ok := parseBucketEnvKey(envKey)
		if !ok {
			continue
		}
		if groups[bucketKey] == nil {
			groups[bucketKey] = map[string]string{}
		}
		groups[bucketKey][field] = value
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("at least one BUCKET_<NAME>_* group is required (e.g. BUCKET_UPLOADS_ENDPOINT)")
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]BucketConfig, 0, len(keys))
	for _, key := range keys {
		b, err := bucketFromEnvGroup(key, groups[key])
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func parseBucketEnvKey(envKey string) (bucketKey, field string, ok bool) {
	rest, ok := strings.CutPrefix(envKey, "BUCKET_")
	if !ok || rest == "" {
		return "", "", false
	}
	for _, f := range bucketEnvFields {
		suffix := "_" + f
		if strings.HasSuffix(rest, suffix) {
			key := strings.TrimSuffix(rest, suffix)
			if key == "" {
				return "", "", false
			}
			return key, f, true
		}
	}
	return "", "", false
}

func bucketFromEnvGroup(key string, fields map[string]string) (BucketConfig, error) {
	prefix := "BUCKET_" + key
	name := strings.TrimSpace(fields["NAME"])
	if name == "" {
		name = strings.ToLower(key)
	}
	b := BucketConfig{
		Name:                name,
		Endpoint:            strings.TrimSpace(fields["ENDPOINT"]),
		PublicEndpoint:      strings.TrimSpace(fields["PUBLICENDPOINT"]),
		AccessKey:           strings.TrimSpace(fields["ACCESSKEY"]),
		SecretKey:           strings.TrimSpace(fields["SECRETKEY"]),
		CDNDomain:           strings.TrimSpace(fields["CDNDOMAIN"]),
		Region:              strings.TrimSpace(fields["REGION"]),
		Addressing:          strings.TrimSpace(fields["ADDRESSING"]),
		AllowedContentTypes: splitList(fields["ALLOWEDCONTENTTYPES"]),
	}
	if b.Endpoint == "" || b.AccessKey == "" || b.SecretKey == "" {
		return BucketConfig{}, fmt.Errorf("%s: ENDPOINT, ACCESSKEY, SECRETKEY are required", prefix)
	}
	if v := strings.TrimSpace(fields["USESSL"]); v != "" {
		useSSL, err := strconv.ParseBool(v)
		if err != nil {
			return BucketConfig{}, fmt.Errorf("%s_USESSL: invalid boolean %q", prefix, v)
		}
		b.UseSSL = useSSL
	}
	if v := strings.TrimSpace(fields["MAXUPLOADBYTES"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return BucketConfig{}, fmt.Errorf("%s_MAXUPLOADBYTES: invalid integer %q", prefix, v)
		}
		if n < 0 {
			return BucketConfig{}, fmt.Errorf("%s_MAXUPLOADBYTES must be >= 0", prefix)
		}
		b.MaxUploadBytes = n
	}
	if err := validateEndpoint(b.Endpoint); err != nil {
		return BucketConfig{}, fmt.Errorf("%s_ENDPOINT: %w", prefix, err)
	}
	if b.PublicEndpoint != "" {
		if err := validateEndpoint(b.PublicEndpoint); err != nil {
			return BucketConfig{}, fmt.Errorf("%s_PUBLICENDPOINT: %w", prefix, err)
		}
	}
	if err := validateAddressing(b.Addressing); err != nil {
		return BucketConfig{}, fmt.Errorf("%s_ADDRESSING: %w", prefix, err)
	}
	if err := rejectCOSBucketHost(b.Endpoint); err != nil {
		return BucketConfig{}, fmt.Errorf("%s_ENDPOINT: %w", prefix, err)
	}
	if b.PublicEndpoint != "" {
		if err := rejectCOSBucketHost(b.PublicEndpoint); err != nil {
			return BucketConfig{}, fmt.Errorf("%s_PUBLICENDPOINT: %w", prefix, err)
		}
	}
	if b.Region == "" {
		b.Region = "us-east-1"
	}
	return b, nil
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
