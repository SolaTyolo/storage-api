package adapter

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SolaTyolo/storage-api/internal/config"
)

func TestPresignUsesPublicEndpointHost(t *testing.T) {
	ad, err := NewMinioAdapter(config.BucketConfig{
		Name:           "uploads",
		Endpoint:       "http://rustfs:9000",
		PublicEndpoint: "http://localhost:9000",
		AccessKey:      "rustfsadmin",
		SecretKey:      "rustfsadmin",
		Region:         "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ad.PresignPut(context.Background(), "uploads", "a.png", "image/png", nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "localhost:9000" {
		t.Fatalf("presign host = %q, want localhost:9000 (must match browser; do not rewrite after signing)", u.Host)
	}
	if !strings.Contains(raw, "X-Amz-Signature=") {
		t.Fatalf("missing signature: %s", raw)
	}
}

func TestPresignFallsBackToEndpoint(t *testing.T) {
	ad, err := NewMinioAdapter(config.BucketConfig{
		Name:      "uploads",
		Endpoint:  "http://rustfs:9000",
		AccessKey: "a",
		SecretKey: "b",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ad.PresignGet(context.Background(), "uploads", "a.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "rustfs:9000" {
		t.Fatalf("host = %q", u.Host)
	}
}
