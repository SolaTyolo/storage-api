package adapter

import "testing"

func TestRewritePublicURL(t *testing.T) {
	a := &MinioAdapter{publicEndpoint: "http://localhost:9000"}
	in := "http://rustfs:9000/uploads/a.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=x"
	out := a.rewritePublicURL(in)
	if out != "http://localhost:9000/uploads/a.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=x" {
		t.Fatalf("unexpected rewrite: %s", out)
	}

	a2 := &MinioAdapter{}
	if a2.rewritePublicURL(in) != in {
		t.Fatal("empty publicEndpoint should keep URL")
	}
}
