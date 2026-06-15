package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("BUCKET_CONFIG", `[{"name":"uploads","endpoint":"http://localhost:9000","accessKey":"a","secretKey":"b","cdnDomain":"http://cdn/uploads","useSSL":false}]`)
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
	_ = os.Getenv // silence if unused in some Go versions
}

func TestLoadRequiresBuckets(t *testing.T) {
	t.Setenv("BUCKET_CONFIG", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}
