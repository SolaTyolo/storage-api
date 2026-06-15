package engine

import "testing"

func TestRewritePublicURL(t *testing.T) {
	e := &S3Engine{publicEndpoint: "http://localhost:9000"}
	raw := "http://rustfs:9000/uploads/key?X-Amz-Signature=abc"
	got := e.rewritePublicURL(raw)
	want := "http://localhost:9000/uploads/key?X-Amz-Signature=abc"
	if got != want {
		t.Fatalf("rewritePublicURL() = %q, want %q", got, want)
	}
}

func TestRewritePublicURLEmpty(t *testing.T) {
	e := &S3Engine{}
	raw := "http://rustfs:9000/uploads/key"
	if got := e.rewritePublicURL(raw); got != raw {
		t.Fatalf("expected unchanged URL, got %q", got)
	}
}
