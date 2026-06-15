package transform

import "testing"

func TestSignImgproxyPathDeterministic(t *testing.T) {
	key := "test-key"
	salt := "test-salt"
	path := "/rs:fit:100:100/plain/czM6Ly9idWNrZXQva2V5"

	sig1 := signImgproxyPath(key, salt, path)
	sig2 := signImgproxyPath(key, salt, path)
	if sig1 != sig2 || sig1 == "" {
		t.Fatalf("unexpected signature %q", sig1)
	}

	sig3 := signImgproxyPath("other", salt, path)
	if sig3 == sig1 {
		t.Fatal("different keys should produce different signatures")
	}
}
