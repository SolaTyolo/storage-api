package config

import (
	"strings"
	"testing"
)

func setUploadsBucket(t *testing.T) {
	t.Helper()
	t.Setenv("BUCKET_UPLOADS_ENDPOINT", "http://localhost:9000")
	t.Setenv("BUCKET_UPLOADS_ACCESSKEY", "a")
	t.Setenv("BUCKET_UPLOADS_SECRETKEY", "b")
	t.Setenv("BUCKET_UPLOADS_CDNDOMAIN", "http://cdn/uploads")
	t.Setenv("BUCKET_UPLOADS_USESSL", "false")
}

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	setUploadsBucket(t)
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

func TestLoadMultipleBuckets(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	setUploadsBucket(t)
	t.Setenv("BUCKET_AVATARS_NAME", "avatars")
	t.Setenv("BUCKET_AVATARS_ENDPOINT", "http://localhost:9001")
	t.Setenv("BUCKET_AVATARS_ACCESSKEY", "x")
	t.Setenv("BUCKET_AVATARS_SECRETKEY", "y")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Buckets) != 2 {
		t.Fatalf("want 2 buckets, got %+v", cfg.Buckets)
	}
	if _, ok := cfg.GetBucket("avatars"); !ok {
		t.Fatal("missing avatars")
	}
}

func TestLoadRequiresBuckets(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when no BUCKET_<NAME>_* vars are set")
	}
}

func TestLoadRequiresSecretUnlessExplicitDevMode(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	setUploadsBucket(t)
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
		"duplicate bucket": {
			"BUCKET_A_NAME":      "u",
			"BUCKET_A_ENDPOINT":  "http://a:9000",
			"BUCKET_A_ACCESSKEY": "a",
			"BUCKET_A_SECRETKEY": "b",
			"BUCKET_B_NAME":      "u",
			"BUCKET_B_ENDPOINT":  "http://b:9000",
			"BUCKET_B_ACCESSKEY": "a",
			"BUCKET_B_SECRETKEY": "b",
		},
		"endpoint with path": {
			"BUCKET_UPLOADS_ENDPOINT": "http://a:9000/s3",
		},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "secret")
			setUploadsBucket(t)
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestUseVirtualHost(t *testing.T) {
	cases := []struct {
		cfg  BucketConfig
		want bool
	}{
		{BucketConfig{Endpoint: "http://rustfs:9000"}, false},
		{BucketConfig{Endpoint: "https://cos.ap-shanghai.myqcloud.com"}, true},
		{BucketConfig{Endpoint: "https://cos.ap-shanghai.myqcloud.com", Addressing: "path"}, false},
		{BucketConfig{Endpoint: "http://rustfs:9000", Addressing: "virtual"}, true},
	}
	for _, tc := range cases {
		if got := tc.cfg.UseVirtualHost(); got != tc.want {
			t.Errorf("%+v UseVirtualHost=%v want %v", tc.cfg, got, tc.want)
		}
	}
}

func TestRejectCOSBucketHost(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("BUCKET_APP_ENDPOINT", "https://develop-1490074736.cos.ap-shanghai.myqcloud.com")
	t.Setenv("BUCKET_APP_ACCESSKEY", "a")
	t.Setenv("BUCKET_APP_SECRETKEY", "b")
	t.Setenv("BUCKET_APP_USESSL", "true")
	t.Setenv("BUCKET_APP_REGION", "ap-shanghai")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "regional COS endpoint") {
		t.Fatalf("expected regional COS endpoint error, got %v", err)
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

func TestParseBucketEnvKey(t *testing.T) {
	key, field, ok := parseBucketEnvKey("BUCKET_UPLOADS_PUBLICENDPOINT")
	if !ok || key != "UPLOADS" || field != "PUBLICENDPOINT" {
		t.Fatalf("got %q %q %v", key, field, ok)
	}
	key, field, ok = parseBucketEnvKey("BUCKET_MY_BUCKET_ACCESSKEY")
	if !ok || key != "MY_BUCKET" || field != "ACCESSKEY" {
		t.Fatalf("got %q %q %v", key, field, ok)
	}
	if _, _, ok := parseBucketEnvKey("BUCKET_CONFIG"); ok {
		t.Fatal("BUCKET_CONFIG must not parse as a bucket field")
	}
}
