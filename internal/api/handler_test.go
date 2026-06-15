package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/preview"
	"github.com/SolaTyolo/storage-api/internal/transform"
)

type mockEngine struct {
	meta    engine.BucketMeta
	objects map[string]engine.ObjectInfo
}

func (m *mockEngine) Name() string { return "mock" }

func (m *mockEngine) CreateBucket(context.Context, string, engine.BucketMeta) error { return nil }
func (m *mockEngine) DeleteBucket(context.Context, string) error                    { return nil }
func (m *mockEngine) ListBuckets(context.Context) ([]string, error)                 { return []string{"uploads"}, nil }
func (m *mockEngine) GetBucketMeta(context.Context, string) (engine.BucketMeta, string, error) {
	return m.meta, "mock-etag", nil
}
func (m *mockEngine) SetBucketMeta(context.Context, string, engine.BucketMeta, string) error {
	return nil
}
func (m *mockEngine) HeadBucket(context.Context, string) error  { return nil }
func (m *mockEngine) EmptyBucket(context.Context, string) error { return nil }

func (m *mockEngine) PutObject(context.Context, string, string, string, string, io.Reader, map[string]string) error {
	return nil
}

func (m *mockEngine) GetObject(_ context.Context, _, key string) (io.ReadCloser, engine.ObjectInfo, error) {
	info, ok := m.objects[key]
	if !ok {
		return nil, engine.ObjectInfo{}, errors.New("NotFound")
	}
	return io.NopCloser(strings.NewReader("payload")), info, nil
}

func (m *mockEngine) HeadObject(_ context.Context, _, key string) (engine.ObjectInfo, error) {
	info, ok := m.objects[key]
	if !ok {
		return engine.ObjectInfo{}, errors.New("NotFound")
	}
	return info, nil
}

func (m *mockEngine) DeleteObject(context.Context, string, string) error    { return nil }
func (m *mockEngine) DeleteObjects(context.Context, string, []string) error { return nil }
func (m *mockEngine) ListObjects(_ context.Context, _, _ string, limit, offset int) ([]engine.ObjectInfo, error) {
	objs := make([]engine.ObjectInfo, 0, len(m.objects))
	for _, info := range m.objects {
		objs = append(objs, info)
	}
	if offset >= len(objs) {
		return nil, nil
	}
	objs = objs[offset:]
	if limit > 0 && len(objs) > limit {
		objs = objs[:limit]
	}
	return objs, nil
}

func (m *mockEngine) ListObjectsV2(_ context.Context, _, _ string, limit int, cursor string, withDelimiter bool) (engine.ListPageV2, error) {
	if cursor != "" {
		return engine.ListPageV2{HasMore: false}, nil
	}
	objs := make([]engine.ObjectInfo, 0, len(m.objects))
	for _, info := range m.objects {
		objs = append(objs, info)
	}
	if len(objs) > limit {
		objs = objs[:limit]
	}
	page := engine.ListPageV2{Objects: objs, HasMore: false}
	if withDelimiter {
		page.Folders = []string{"nested/"}
	}
	return page, nil
}

func (m *mockEngine) CopyObject(context.Context, string, string, string, string) error { return nil }

func (m *mockEngine) PresignGet(_ context.Context, bucket, key string, _ time.Duration, _ string) (string, error) {
	return "https://s3.example.com/" + bucket + "/" + key + "?presigned=get", nil
}

func (m *mockEngine) PresignPut(_ context.Context, bucket, key, _ string, _ time.Duration) (string, error) {
	return "https://s3.example.com/" + bucket + "/" + key + "?presigned=put", nil
}

func newTestHandler(t *testing.T, cfg config.Config, eng engine.Engine) http.Handler {
	t.Helper()
	reg, err := engine.NewRegistry("mock", map[string]engine.Engine{"mock": eng})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewRouter(cfg, reg, transform.New(cfg, reg), preview.New(reg, "", "", ""), log)
}

func defaultMockEngine() *mockEngine {
	return &mockEngine{
		meta: engine.BucketMeta{Public: true},
		objects: map[string]engine.ObjectInfo{
			"foo.jpg": {Path: "foo.jpg", ContentType: "image/jpeg", Size: 6, ETag: "abc"},
		},
	}
}

func testConfig() config.Config {
	return config.Config{
		APIKey:               "test-key",
		PresignExpires:       time.Hour,
		PublicPresignExpires: 15 * time.Minute,
		AllowPresignedUpload: true,
		AuthDownloadMode:     "proxy",
		TransformBackend:     "internal",
		TransformMaxEdge:     4096,
	}
}

func TestSignObjectReturnsPresignedURL(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/sign/uploads/foo.jpg", nil)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "https://s3.example.com/uploads/foo.jpg?presigned=get") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestPublicGetRedirects(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodGet, "/storage/v1/object/public/uploads/foo.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status %d want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://s3.example.com/uploads/foo.jpg") {
		t.Fatalf("location %q", loc)
	}
}

func TestPublicHeadReturnsMetadata(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodHead, "/storage/v1/object/public/uploads/foo.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("content-type %q", rr.Header().Get("Content-Type"))
	}
}

func TestPublicGetForbiddenWhenNotPublic(t *testing.T) {
	eng := defaultMockEngine()
	eng.meta.Public = false
	h := newTestHandler(t, testConfig(), eng)
	req := httptest.NewRequest(http.MethodGet, "/storage/v1/object/public/uploads/foo.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestSignUploadRejectedByMimePolicy(t *testing.T) {
	eng := defaultMockEngine()
	eng.meta.AllowedMimeTypes = []string{"image/*"}
	h := newTestHandler(t, testConfig(), eng)

	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/upload/sign/uploads/video.mp4", nil)
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "video/mp4")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestSignUploadDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.AllowPresignedUpload = false
	h := newTestHandler(t, cfg, defaultMockEngine())

	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/upload/sign/uploads/foo.jpg", nil)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestLegacyDownloadTokenGone(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodGet, "/storage/v1/object/sign/uploads/foo.jpg?token=legacy", nil)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusGone {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestAuthDownloadRedirectMode(t *testing.T) {
	cfg := testConfig()
	cfg.AuthDownloadMode = "redirect"
	h := newTestHandler(t, cfg, defaultMockEngine())

	req := httptest.NewRequest(http.MethodGet, "/storage/v1/object/uploads/foo.jpg", nil)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status %d want 302", rr.Code)
	}
}

func TestUploadRejectedByMimePolicy(t *testing.T) {
	eng := defaultMockEngine()
	eng.meta.AllowedMimeTypes = []string{"image/*"}
	h := newTestHandler(t, testConfig(), eng)

	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/uploads/video.mp4", strings.NewReader("data"))
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("x-upsert", "true")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestListObjectsV2(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	body := strings.NewReader(`{"prefix":"","limit":10}`)
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/list-v2/uploads", body)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"hasNext"`) || !strings.Contains(rr.Body.String(), `"objects"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestListObjectsV2WithDelimiter(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	body := strings.NewReader(`{"with_delimiter":true}`)
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/list-v2/uploads", body)
	req.Header.Set("apikey", "test-key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"folders"`) {
		t.Fatalf("expected folders in body: %s", rr.Body.String())
	}
}
