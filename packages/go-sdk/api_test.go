package objsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGatewayAPIPresignPut(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/presign/put" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["bucket"] != "uploads" || body["key"] != "a.png" {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"uploadUrl": "https://s3/put",
			"cdnUrl":    "https://cdn/a.png",
		})
	}))
	t.Cleanup(srv.Close)

	api := newGatewayAPI(srv.URL+"/api/v1", "tok", srv.Client())
	res, err := api.presignPut(context.Background(), bucketKeyExpire{
		Bucket:      "uploads",
		Key:         "a.png",
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.UploadURL != "https://s3/put" || res.CDNURL != "https://cdn/a.png" {
		t.Fatalf("res = %#v", res)
	}
}

func TestGatewayAPIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","message":"key prefix not allowed"}`))
	}))
	t.Cleanup(srv.Close)

	api := newGatewayAPI(srv.URL+"/api/v1", "tok", srv.Client())
	_, err := api.objectHead(context.Background(), "uploads", "x")
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err type = %T", err)
	}
	if ae.Status != 403 || ae.Message != "key prefix not allowed" {
		t.Fatalf("ae = %#v", ae)
	}
}

func TestClientUpload(t *testing.T) {
	t.Parallel()
	var putBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/presign/put", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"uploadUrl": "http://" + r.Host + "/s3/put",
			"cdnUrl":    "https://cdn/report.pdf",
		})
	})
	mux.HandleFunc("/s3/put", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/pdf" {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("x-amz-meta-source") != "billing" {
			t.Fatalf("meta = %q", r.Header.Get("x-amz-meta-source"))
		}
		var err error
		putBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/object/head", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"size":        5,
			"contentType": "application/pdf",
			"meta":        map[string]string{"source": "billing"},
			"etag":        "abc123",
			"cdnUrl":      "https://cdn/report.pdf",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := New(Config{
		Endpoint:      srv.URL + "/api/v1",
		Token:         "tok",
		DefaultBucket: "uploads",
		HTTPClient:    srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := c.UploadBytes(context.Background(), []byte("hello"), UploadOptions{
		Key:         "reports/a.pdf",
		ContentType: "application/pdf",
		Meta:        Meta{"source": "billing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(putBody) != "hello" {
		t.Fatalf("putBody = %q", putBody)
	}
	if res.Key != "reports/a.pdf" || res.ETag != "abc123" || res.CDNURL != "https://cdn/report.pdf" {
		t.Fatalf("res = %#v", res)
	}
}

func TestClientDownloadURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/presign/get") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"downloadUrl": "https://s3/get"})
	}))
	t.Cleanup(srv.Close)

	c, err := New(Config{Endpoint: srv.URL + "/api/v1", Token: "tok", DefaultBucket: "uploads", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	url, err := c.DownloadURL(context.Background(), "private/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://s3/get" {
		t.Fatalf("url = %q", url)
	}
}

func TestClientMultipartUpload(t *testing.T) {
	t.Parallel()
	parts := map[int][]byte{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/multipart/create", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"uploadId": "uid-1"})
	})
	mux.HandleFunc("/api/v1/multipart/presign-part", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PartNumber int `json:"partNumber"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"partUploadUrl": "http://" + r.Host + "/s3/part/" + strconv.Itoa(body.PartNumber),
		})
	})
	mux.HandleFunc("/s3/part/", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/s3/part/"))
		b, _ := io.ReadAll(r.Body)
		parts[n] = b
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(n)+`"`)
	})
	mux.HandleFunc("/api/v1/multipart/complete", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"etag": "final"})
	})
	mux.HandleFunc("/api/v1/object/head", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"size": 6, "contentType": "application/octet-stream",
			"meta": map[string]string{}, "etag": "final", "cdnUrl": "https://cdn/x",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := New(Config{Endpoint: srv.URL + "/api/v1", Token: "tok", DefaultBucket: "uploads", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("abcdef")
	res, err := c.MultipartUpload(context.Background(), bytes.NewReader(data), int64(len(data)), MultipartUploadOptions{
		UploadOptions: UploadOptions{Key: "big.bin"},
		PartSize:      5 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ETag != "final" {
		t.Fatalf("res = %#v", res)
	}
	if string(parts[1]) != "abcdef" {
		t.Fatalf("parts = %#v", parts)
	}
}
