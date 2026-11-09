package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/SolaTyolo/storage-api/internal/adapter"
	"github.com/SolaTyolo/storage-api/internal/auth"
	"github.com/SolaTyolo/storage-api/internal/config"
)

const testSecret = "test-secret"

type fakeAdapter struct {
	objects  map[string]*adapter.ObjectInfo
	lastSize int64
	deleted  []string
	pingErr  error
}

func newFake() *fakeAdapter {
	return &fakeAdapter{objects: map[string]*adapter.ObjectInfo{}}
}

func (f *fakeAdapter) PresignPut(_ context.Context, bucket, key, _ string, size int64, _ map[string]string, expiry time.Duration) (string, error) {
	f.lastSize = size
	return fmt.Sprintf("https://s3/%s/%s?put&exp=%d", bucket, key, int(expiry.Seconds())), nil
}

func (f *fakeAdapter) CreateMultipart(context.Context, string, string, string, map[string]string) (string, error) {
	return "upload-1", nil
}

func (f *fakeAdapter) PresignUploadPart(_ context.Context, _, key, uploadID string, n int, _ time.Duration) (string, error) {
	return fmt.Sprintf("https://s3/%s?uploadId=%s&partNumber=%d", key, uploadID, n), nil
}

func (f *fakeAdapter) CompleteMultipart(_ context.Context, _, key, uploadID string, _ []adapter.CompletePart) (string, error) {
	if uploadID == "missing" {
		return "", fmt.Errorf("complete multipart: %w: NoSuchUpload", adapter.ErrNotFound)
	}
	return "final-etag", nil
}

func (f *fakeAdapter) AbortMultipart(context.Context, string, string, string) error { return nil }

func (f *fakeAdapter) PresignGet(_ context.Context, _, key string, _ time.Duration) (string, error) {
	return "https://s3/" + key + "?get", nil
}

func (f *fakeAdapter) HeadObject(_ context.Context, _, key string) (*adapter.ObjectInfo, error) {
	info, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("head object: %w: NoSuchKey", adapter.ErrNotFound)
	}
	return info, nil
}

func (f *fakeAdapter) ListObjects(_ context.Context, _, prefix, _, _ string, maxKeys int) (*adapter.ListResult, error) {
	res := &adapter.ListResult{}
	for k, v := range f.objects {
		if strings.HasPrefix(k, prefix) && len(res.Entries) < maxKeys {
			res.Entries = append(res.Entries, adapter.ListEntry{Key: k, Size: v.Size, ETag: v.ETag})
		}
	}
	return res, nil
}

func (f *fakeAdapter) DeleteObject(_ context.Context, _, key string) error {
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

func (f *fakeAdapter) CopyObject(_ context.Context, _, src, dst string) error {
	info, ok := f.objects[src]
	if !ok {
		return fmt.Errorf("copy object: %w", adapter.ErrNotFound)
	}
	f.objects[dst] = info
	return nil
}

func (f *fakeAdapter) CDNURL(_, key string) string { return "https://cdn/" + key }

func (f *fakeAdapter) Ping(context.Context, string) error { return f.pingErr }

type testEnv struct {
	srv  *httptest.Server
	fake *fakeAdapter
}

func newEnv(t *testing.T, mutate func(*config.Config)) *testEnv {
	t.Helper()
	cfg := &config.Config{
		DefaultExpire:      15 * time.Minute,
		MaxExpire:          time.Hour,
		CORSAllowedOrigins: []string{"*"},
		MaxBodyBytes:       4096,
		BucketByName: map[string]config.BucketConfig{
			"uploads": {Name: "uploads"},
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	fake := newFake()
	reg := adapter.NewRegistryFrom(map[string]adapter.ObjectAdapter{"uploads": fake})
	h := New(cfg, reg, auth.New(auth.Options{Secret: testSecret}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(h.Router())
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, fake: fake}
}

func token(t *testing.T, c auth.Claims) string {
	t.Helper()
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &c).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var serviceClaims = auth.Claims{Role: "service_role"}

func (e *testEnv) post(t *testing.T, tok, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf []byte
	switch b := body.(type) {
	case string:
		buf = []byte(b)
	default:
		var err error
		if buf, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1"+path, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestRequiresToken(t *testing.T) {
	e := newEnv(t, nil)
	status, body := e.post(t, "", "/presign/get", map[string]any{"bucket": "uploads", "key": "a"})
	if status != http.StatusUnauthorized || body["error"] != "unauthorized" {
		t.Fatalf("status=%d body=%v", status, body)
	}
}

func TestPresignPut(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, auth.Claims{AllowedPrefixes: []string{"users/1"}})

	status, body := e.post(t, tok, "/presign/put", map[string]any{"bucket": "uploads", "key": "users/1/a.png", "size": 10})
	if status != http.StatusOK || body["cdnUrl"] != "https://cdn/users/1/a.png" {
		t.Fatalf("status=%d body=%v", status, body)
	}
	if e.fake.lastSize != 10 {
		t.Fatalf("signed size = %d, want 10", e.fake.lastSize)
	}

	status, _ = e.post(t, tok, "/presign/put", map[string]any{"bucket": "uploads", "key": "users/1/b.png"})
	if status != http.StatusOK || e.fake.lastSize != -1 {
		t.Fatalf("status=%d size=%d, want unsigned size", status, e.fake.lastSize)
	}

	status, _ = e.post(t, tok, "/presign/put", map[string]any{"bucket": "uploads", "key": "users/10/a.png"})
	if status != http.StatusForbidden {
		t.Fatalf("sibling prefix status=%d, want 403", status)
	}
}

func TestKeyValidation(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, serviceClaims)
	for _, key := range []string{"", "/abs", "a//b", "a/../b", "./a", "a/.", "a\nb", strings.Repeat("k", 1025)} {
		status, body := e.post(t, tok, "/presign/get", map[string]any{"bucket": "uploads", "key": key})
		if status != http.StatusBadRequest {
			t.Errorf("key %q: status=%d body=%v, want 400", key, status, body)
		}
	}
	status, _ := e.post(t, tok, "/presign/get", map[string]any{"bucket": "uploads", "key": "folder/"})
	if status != http.StatusOK {
		t.Fatalf("folder marker key status=%d", status)
	}
}

func TestExpireLimits(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, serviceClaims)
	for exp, want := range map[int]int{0: 200, 3600: 200, 3601: 400, -1: 400} {
		status, body := e.post(t, tok, "/presign/get", map[string]any{"bucket": "uploads", "key": "a", "expireSec": exp})
		if status != want {
			t.Errorf("expireSec=%d status=%d body=%v, want %d", exp, status, body, want)
		}
	}
}

func TestUploadPolicy(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.BucketByName["uploads"] = config.BucketConfig{
			Name: "uploads", MaxUploadBytes: 100, AllowedContentTypes: []string{"image/*"},
		}
	})
	tok := token(t, serviceClaims)
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"ok", map[string]any{"contentType": "image/png", "size": 100}, 200},
		{"missing size", map[string]any{"contentType": "image/png"}, 400},
		{"too large", map[string]any{"contentType": "image/png", "size": 101}, 413},
		{"bad type", map[string]any{"contentType": "text/html", "size": 1}, 415},
		{"negative size", map[string]any{"contentType": "image/png", "size": -1}, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["bucket"], tc.body["key"] = "uploads", "a.png"
			if status, body := e.post(t, tok, "/presign/put", tc.body); status != tc.want {
				t.Fatalf("status=%d body=%v, want %d", status, body, tc.want)
			}
			if status, body := e.post(t, tok, "/multipart/create", tc.body); status != tc.want {
				t.Fatalf("multipart status=%d body=%v, want %d", status, body, tc.want)
			}
		})
	}
}

func TestMultipartCompleteEnforcesMaxSize(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.BucketByName["uploads"] = config.BucketConfig{Name: "uploads", MaxUploadBytes: 100}
	})
	e.fake.objects["big.bin"] = &adapter.ObjectInfo{Size: 500}
	tok := token(t, serviceClaims)
	status, _ := e.post(t, tok, "/multipart/complete", map[string]any{
		"bucket": "uploads", "key": "big.bin", "uploadId": "u",
		"parts": []map[string]any{{"partNumber": 1, "etag": "e1"}},
	})
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want 413", status)
	}
	if len(e.fake.deleted) != 1 || e.fake.deleted[0] != "big.bin" {
		t.Fatalf("oversized object not deleted: %v", e.fake.deleted)
	}
}

func TestMultipartValidation(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, serviceClaims)
	base := func(extra map[string]any) map[string]any {
		m := map[string]any{"bucket": "uploads", "key": "a.bin", "uploadId": "u"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if status, _ := e.post(t, tok, "/multipart/presign-part", base(map[string]any{"partNumber": 10001})); status != 400 {
		t.Fatalf("partNumber 10001 status=%d", status)
	}
	if status, _ := e.post(t, tok, "/multipart/complete", base(map[string]any{
		"parts": []map[string]any{{"partNumber": 1}},
	})); status != 400 {
		t.Fatalf("missing etag status=%d", status)
	}
	status, body := e.post(t, tok, "/multipart/complete", map[string]any{
		"bucket": "uploads", "key": "a.bin", "uploadId": "missing",
		"parts": []map[string]any{{"partNumber": 1, "etag": "e"}},
	})
	if status != http.StatusNotFound {
		t.Fatalf("missing upload status=%d body=%v", status, body)
	}
}

func TestMultipartPresignParts(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, serviceClaims)
	status, body := e.post(t, tok, "/multipart/presign-parts", map[string]any{
		"bucket": "uploads", "key": "a.bin", "uploadId": "u", "partNumbers": []int{1, 2, 3},
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}
	parts, _ := body["parts"].([]any)
	if len(parts) != 3 {
		t.Fatalf("parts=%v", body["parts"])
	}
	first := parts[0].(map[string]any)
	if first["partNumber"].(float64) != 1 || !strings.Contains(first["url"].(string), "partNumber=1") {
		t.Fatalf("first part = %v", first)
	}
	if status, _ := e.post(t, tok, "/multipart/presign-parts", map[string]any{
		"bucket": "uploads", "key": "a.bin", "uploadId": "u", "partNumbers": []int{},
	}); status != 400 {
		t.Fatalf("empty batch status=%d", status)
	}
}

func TestScopes(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.objects["a"] = &adapter.ObjectInfo{Size: 1}
	readOnly := token(t, auth.Claims{Scopes: []string{auth.ScopeRead}})
	if status, _ := e.post(t, readOnly, "/object/head", map[string]any{"bucket": "uploads", "key": "a"}); status != 200 {
		t.Fatalf("read-only head status=%d", status)
	}
	if status, _ := e.post(t, readOnly, "/object/delete", map[string]any{"bucket": "uploads", "key": "a"}); status != 403 {
		t.Fatalf("read-only delete status=%d", status)
	}
	if status, _ := e.post(t, readOnly, "/object/copy", map[string]any{"bucket": "uploads", "srcKey": "a", "destKey": "b"}); status != 403 {
		t.Fatalf("read-only copy status=%d", status)
	}
}

func TestErrorMapping(t *testing.T) {
	e := newEnv(t, nil)
	tok := token(t, serviceClaims)
	status, body := e.post(t, tok, "/object/head", map[string]any{"bucket": "uploads", "key": "nope"})
	if status != http.StatusNotFound || body["error"] != "not_found" {
		t.Fatalf("head status=%d body=%v", status, body)
	}
	status, _ = e.post(t, tok, "/object/copy", map[string]any{"bucket": "uploads", "srcKey": "nope", "destKey": "b"})
	if status != http.StatusNotFound {
		t.Fatalf("copy status=%d", status)
	}
	status, body = e.post(t, tok, "/object/head", "{not json")
	if status != http.StatusBadRequest {
		t.Fatalf("bad json status=%d body=%v", status, body)
	}
	status, body = e.post(t, tok, "/object/head", map[string]any{"bucket": "uploads", "key": strings.Repeat("x", 5000)})
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status=%d body=%v", status, body)
	}
	status, body = e.post(t, tok, "/object/head", map[string]any{"bucket": "other", "key": "a"})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown bucket status=%d body=%v", status, body)
	}
}

func TestObjectList(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.objects["users/1/a"] = &adapter.ObjectInfo{Size: 3, ETag: "e1"}
	tok := token(t, auth.Claims{AllowedPrefixes: []string{"users/1/"}})

	status, body := e.post(t, tok, "/object/list", map[string]any{"bucket": "uploads", "prefix": "users/1/"})
	if status != 200 {
		t.Fatalf("status=%d body=%v", status, body)
	}
	entries := body["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["etag"] != "e1" {
		t.Fatalf("entries=%v", entries)
	}
	if status, _ := e.post(t, tok, "/object/list", map[string]any{"bucket": "uploads"}); status != 403 {
		t.Fatalf("root list status=%d, want 403", status)
	}
	if status, _ := e.post(t, tok, "/object/list", map[string]any{"bucket": "uploads", "prefix": "users/1"}); status != 403 {
		t.Fatalf("prefix without boundary status=%d, want 403", status)
	}
}

func TestReady(t *testing.T) {
	e := newEnv(t, nil)
	res, err := e.srv.Client().Get(e.srv.URL + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("ready status=%d", res.StatusCode)
	}
	e.fake.pingErr = fmt.Errorf("down")
	res, err = e.srv.Client().Get(e.srv.URL + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("ready status=%d, want 503", res.StatusCode)
	}
	if res.Header.Get("X-Request-Id") == "" {
		t.Fatal("missing X-Request-Id header")
	}
}
