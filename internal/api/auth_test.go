package api

import (
	"net/http/httptest"
	"testing"
)

func TestAuthExemptPublicObject(t *testing.T) {
	r := httptest.NewRequest("GET", "/storage/v1/object/public/uploads/foo.jpg", nil)
	if !authExempt(r) {
		t.Fatal("public GET should be exempt")
	}
	r = httptest.NewRequest("POST", "/storage/v1/object/public/uploads/foo.jpg", nil)
	if authExempt(r) {
		t.Fatal("public POST should not be exempt")
	}
}

func TestAuthExemptRenderPublic(t *testing.T) {
	r := httptest.NewRequest("HEAD", "/storage/v1/render/image/public/uploads/x.jpg", nil)
	if !authExempt(r) {
		t.Fatal("public render HEAD should be exempt")
	}
}

func TestAuthExemptLegacySignTokenRemoved(t *testing.T) {
	r := httptest.NewRequest("GET", "/storage/v1/object/sign/uploads/foo.jpg?token=abc", nil)
	if authExempt(r) {
		t.Fatal("legacy sign token path should require API_KEY")
	}
}

func TestMatchAPIKeyMultiple(t *testing.T) {
	keys := []string{"key-a", "key-b"}
	if !matchAPIKey("key-b", keys) {
		t.Fatal("expected key-b to match")
	}
	if matchAPIKey("wrong", keys) {
		t.Fatal("expected wrong key to fail")
	}
}
