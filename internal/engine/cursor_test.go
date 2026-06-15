package engine

import "testing"

func TestListCursorRoundTrip(t *testing.T) {
	token := "continuation-token-abc123"
	enc := encodeListCursor(token)
	got, err := decodeListCursor(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != token {
		t.Fatalf("got %q want %q", got, token)
	}
}

func TestDecodeListCursorInvalid(t *testing.T) {
	if _, err := decodeListCursor("not-valid!!!"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}
