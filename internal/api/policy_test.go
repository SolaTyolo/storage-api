package api

import (
	"testing"

	"github.com/SolaTyolo/storage-api/internal/engine"
)

func TestMimeAllowed(t *testing.T) {
	allowed := []string{"image/*", "application/pdf"}
	if !mimeAllowed("image/png", allowed) {
		t.Fatal("image/png should match image/*")
	}
	if !mimeAllowed("application/pdf", allowed) {
		t.Fatal("application/pdf should match")
	}
	if mimeAllowed("video/mp4", allowed) {
		t.Fatal("video/mp4 should not match")
	}
	if !mimeAllowed("anything", []string{"*/*"}) {
		t.Fatal("*/* should allow all")
	}
}

func TestValidateUploadPolicySize(t *testing.T) {
	limit := int64(100)
	resolved := engine.ResolvedBucket{FileSizeLimit: &limit}
	if err := validateUploadPolicy(resolved, "image/png", 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateUploadPolicy(resolved, "image/png", 200); err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestValidateUploadPolicyMime(t *testing.T) {
	resolved := engine.ResolvedBucket{AllowedMimeTypes: []string{"image/*"}}
	if err := validateUploadPolicy(resolved, "image/png", 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateUploadPolicy(resolved, "video/mp4", 10); err == nil {
		t.Fatal("expected mime error")
	}
}
