package config

import (
	"strings"
	"testing"
)

const testBuckets = `[{"name":"uploads","endpoint":"http://localhost:9000","accessKey":"a","secretKey":"b","cdnDomain":"http://cdn/uploads","useSSL":false}]`

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("BUCKET_CONFIG", testBuckets)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Buckets) != 1 || cfg.Buckets[0].Name != "uploads" {
		t.Fatalf("unexpected buckets: %+v", cfg.Buckets)
	}
	if _, ok := cfg.GetBucket("uploads"); !ok {
		t.Fatal("GetBucket failed")
	}
	if cfg.AllowNoJWT {
		t.Fatal("AllowNoJWT must be false when JWT_SECRET is set")
	}
}

func TestLoadRequiresBuckets(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("BUCKET_CONFIG", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRequiresSecretUnlessExplicitDevMode(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("BUCKET_CONFIG", testBuckets)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected JWT_SECRET error, got %v", err)
	}

	t.Setenv("ALLOW_NO_JWT", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowNoJWT {
		t.Fatal("expected AllowNoJWT")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"bad int":         {"DEFAULT_EXPIRE_SEC": "abc"},
		"default > max":   {"DEFAULT_EXPIRE_SEC": "7200", "MAX_EXPIRE_SEC": "3600"},
		"max over s3 cap": {"MAX_EXPIRE_SEC": "999999999"},
		"duplicate bucket": {"BUCKET_CONFIG": `[` +
			`{"name":"u","endpoint":"http://a:9000","accessKey":"a","secretKey":"b"},` +
			`{"name":"u","endpoint":"http://b:9000","accessKey":"a","secretKey":"b"}]`},
		"endpoint with path": {"BUCKET_CONFIG": `[{"name":"u","endpoint":"http://a:9000/s3","accessKey":"a","secretKey":"b"}]`},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "secret")
			t.Setenv("BUCKET_CONFIG", testBuckets)
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestContentTypeAllowed(t *testing.T) {
	b := BucketConfig{AllowedContentTypes: []string{"image/*", "application/pdf"}}
	for ct, want := range map[string]bool{
		"image/png":                 true,
		"IMAGE/JPEG":                true,
		"application/pdf":           true,
		"application/pdf; charset=": true,
		"text/html":                 false,
		"imagex/png":                false,
	} {
		if got := b.ContentTypeAllowed(ct); got != want {
			t.Errorf("ContentTypeAllowed(%q) = %v, want %v", ct, got, want)
		}
	}
	if !(BucketConfig{}).ContentTypeAllowed("anything/else") {
		t.Fatal("empty allowlist must allow all")
	}
}
