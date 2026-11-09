package adapter

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/minio/minio-go/v7"
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
	raw, err := ad.PresignPut(context.Background(), "uploads", "a.png", "image/png", -1, nil, time.Minute)
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

func TestCOSUsesVirtualHostedPresign(t *testing.T) {
	ad, err := NewMinioAdapter(config.BucketConfig{
		Name:           "develop-1490074736",
		Endpoint:       "https://cos.ap-shanghai.myqcloud.com",
		PublicEndpoint: "https://cos.ap-shanghai.myqcloud.com",
		AccessKey:      "AKIDtest",
		SecretKey:      "secret",
		Region:         "ap-shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ad.PresignPut(context.Background(), "develop-1490074736", "demo/a.png", "image/png", 10, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	wantHost := "develop-1490074736.cos.ap-shanghai.myqcloud.com"
	if u.Host != wantHost {
		t.Fatalf("presign host = %q, want %q", u.Host, wantHost)
	}
	if strings.Contains(u.EscapedPath(), "/develop-1490074736/") {
		t.Fatalf("bucket must not appear in object path: %s", u.Path)
	}
	if !strings.HasPrefix(u.EscapedPath(), "/demo/") {
		t.Fatalf("object path = %q, want /demo/...", u.Path)
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

func TestPresignPutSignsContentLength(t *testing.T) {
	ad, err := NewMinioAdapter(config.BucketConfig{
		Name: "uploads", Endpoint: "http://rustfs:9000", AccessKey: "a", SecretKey: "b", Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ad.PresignPut(context.Background(), "uploads", "a.png", "image/png", 1234, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	signed := u.Query().Get("X-Amz-SignedHeaders")
	if !strings.Contains(signed, "content-length") || !strings.Contains(signed, "content-type") {
		t.Fatalf("X-Amz-SignedHeaders = %q, want content-length and content-type", signed)
	}

	raw, err = ad.PresignPut(context.Background(), "uploads", "a.png", "image/png", -1, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "content-length") {
		t.Fatalf("unknown size must not sign content-length: %s", raw)
	}
}

func TestCDNURLEscapesKey(t *testing.T) {
	ad := &MinioAdapter{cdnDomain: "https://cdn.example/uploads"}
	got := ad.CDNURL("uploads", "a dir/b#1?.png")
	want := "https://cdn.example/uploads/a%20dir/b%231%3F.png"
	if got != want {
		t.Fatalf("CDNURL = %q, want %q", got, want)
	}
	if (&MinioAdapter{}).CDNURL("uploads", "x") != "" {
		t.Fatal("empty cdnDomain must return empty URL")
	}
}

func TestWrapErrKinds(t *testing.T) {
	cases := map[string]error{
		"NoSuchKey":    ErrNotFound,
		"NoSuchUpload": ErrNotFound,
		"AccessDenied": ErrAccessDenied,
		"InvalidPart":  ErrInvalid,
	}
	for code, want := range cases {
		err := wrapErr("op", minio.ErrorResponse{Code: code, StatusCode: 400})
		if !errors.Is(err, want) {
			t.Errorf("%s: got %v, want kind %v", code, err, want)
		}
	}
	if err := wrapErr("op", errors.New("boom")); errors.Is(err, ErrNotFound) {
		t.Fatal("plain error must not get a kind")
	}
}
