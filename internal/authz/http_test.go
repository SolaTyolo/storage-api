package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPAuthorizerAllows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Resource.BucketID != "uploads" || req.Resource.Action != ActionRead {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(Response{Allowed: false, Reason: "nope"})
			return
		}
		_ = json.NewEncoder(w).Encode(Response{Allowed: true})
	}))
	defer srv.Close()

	az := &HTTP{URL: srv.URL, TimeoutSec: 2}
	err := az.Authorize(context.Background(), Subject{Type: "jwt", Role: "authenticated"}, Resource{
		Method: "GET", Path: "/storage/v1/object/authenticated/uploads/a.jpg", Action: ActionRead, BucketID: "uploads", ObjectPath: "a.jpg",
	})
	if err != nil {
		t.Fatalf("expected allow: %v", err)
	}
}

func TestHTTPAuthorizerDenies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(Response{Allowed: false, Reason: "policy"})
	}))
	defer srv.Close()

	az := &HTTP{URL: srv.URL, TimeoutSec: 2}
	err := az.Authorize(context.Background(), Subject{Type: "jwt"}, Resource{Action: ActionWrite, BucketID: "secret"})
	if err == nil {
		t.Fatal("expected deny")
	}
}

func TestParseRequestObjectRead(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/storage/v1/object/authenticated/uploads/foo.jpg", nil)
	action, bucket, path := ParseRequest(r)
	if action != ActionRead || bucket != "uploads" || path != "foo.jpg" {
		t.Fatalf("got %s %s %s", action, bucket, path)
	}
}
