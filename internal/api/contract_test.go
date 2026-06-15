package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Contract tests mirror @supabase/storage-js HTTP expectations (StorageFileApi / StorageBucketApi).

func TestContractStorageErrorShape(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodGet, "/storage/v1/object/authenticated/uploads/missing.jpg", nil)
	req.Header.Set("apikey", "test-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	var errBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"statusCode", "error", "message"} {
		if errBody[k] == "" {
			t.Fatalf("missing %s in %#v", k, errBody)
		}
	}
}

func TestContractListObjects(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	body := bytes.NewBufferString(`{"prefix":"","limit":100,"offset":0}`)
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/list/uploads", body)
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 object, got %d", len(out))
	}
	for _, k := range []string{"name", "id", "updated_at", "created_at", "metadata"} {
		if _, ok := out[0][k]; !ok {
			t.Fatalf("list item missing %s", k)
		}
	}
}

func TestContractListV2Shape(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	body := bytes.NewBufferString(`{"prefix":"","limit":100}`)
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/list-v2/uploads", body)
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"objects", "folders", "hasNext"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("list-v2 missing %s", k)
		}
	}
}

func TestContractCreateSignedUrl(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/sign/uploads/foo.jpg", strings.NewReader(`{"expiresIn":3600}`))
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["signedURL"] == "" || out["path"] != "foo.jpg" {
		t.Fatalf("unexpected %#v", out)
	}
	if !strings.HasPrefix(out["signedURL"], "https://") {
		t.Fatalf("signedURL should be absolute S3 URL, got %q", out["signedURL"])
	}
}

func TestContractCopyWithDestinationBucket(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	payload := `{"bucketId":"uploads","sourceKey":"foo.jpg","destinationKey":"bar.jpg","destinationBucket":"uploads"}`
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/copy", strings.NewReader(payload))
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["Key"] != "bar.jpg" {
		t.Fatalf("unexpected %#v", out)
	}
}

func TestContractUploadUsesMultipart(t *testing.T) {
	h := newTestHandler(t, testConfig(), defaultMockEngine())
	var buf bytes.Buffer
	buf.WriteString("--bound\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.txt\"\r\nContent-Type: text/plain\r\n\r\nhello\r\n--bound--\r\n")
	req := httptest.NewRequest(http.MethodPost, "/storage/v1/object/uploads/new.txt", &buf)
	req.Header.Set("apikey", "test-key")
	req.Header.Set("Content-Type", "multipart/form-data; boundary=bound")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	raw, _ := io.ReadAll(rec.Body)
	if !bytes.Contains(raw, []byte(`"Key"`)) && !bytes.Contains(raw, []byte("new.txt")) {
		t.Fatalf("unexpected body %s", raw)
	}
}

func TestContractAuthzDenied(t *testing.T) {
	cfg := testConfig()
	cfg.AuthzHTTPURL = "" // noop by default
	cfg.AuthzBypassAPIKey = false
	// use fake HTTP authz via httptest in authz package; here ensure middleware present with noop
	h := newTestHandler(t, cfg, defaultMockEngine())
	req := httptest.NewRequest(http.MethodGet, "/storage/v1/bucket", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without apikey, got %d", rec.Code)
	}
}
