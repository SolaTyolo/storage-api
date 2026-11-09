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
	"sync"
	"testing"
)

type fakeStorage struct {
	mu          sync.Mutex
	parts       map[int][]byte
	failOnce    map[int]bool
	completed   []CompletePart
	createSize  *int64
	aborted     bool
	signBatches [][]int
}

func newFakeStorage(t *testing.T) (*fakeStorage, *httptest.Server) {
	t.Helper()
	fs := &fakeStorage{parts: map[int][]byte{}, failOnce: map[int]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/multipart/create", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Size *int64 `json:"size"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fs.mu.Lock()
		fs.createSize = body.Size
		fs.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"uploadId": "uid-1"})
	})
	mux.HandleFunc("/api/v1/multipart/presign-parts", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PartNumbers []int `json:"partNumbers"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fs.mu.Lock()
		fs.signBatches = append(fs.signBatches, body.PartNumbers)
		fs.mu.Unlock()
		parts := make([]SignedPart, 0, len(body.PartNumbers))
		for _, n := range body.PartNumbers {
			parts = append(parts, SignedPart{PartNumber: n, URL: "http://" + r.Host + "/s3/part/" + strconv.Itoa(n)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"parts": parts})
	})
	mux.HandleFunc("/s3/part/", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/s3/part/"))
		b, _ := io.ReadAll(r.Body)
		fs.mu.Lock()
		defer fs.mu.Unlock()
		if fs.failOnce[n] {
			fs.failOnce[n] = false
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fs.parts[n] = b
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(n)+`"`)
	})
	mux.HandleFunc("/api/v1/multipart/complete", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Parts []CompletePart `json:"parts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fs.mu.Lock()
		fs.completed = body.Parts
		fs.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"etag": "final"})
	})
	mux.HandleFunc("/api/v1/multipart/abort", func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.aborted = true
		fs.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/object/head", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"etag": "final", "cdnUrl": "https://cdn/x"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fs, srv
}

func (fs *fakeStorage) assembled() []byte {
	var out []byte
	for _, p := range fs.completed {
		out = append(out, fs.parts[p.PartNumber]...)
	}
	return out
}

func testData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestMultipartUploadConcurrentWithRetry(t *testing.T) {
	t.Parallel()
	data := testData(int(minPartSize)*2 + 1234)
	cases := map[string]func() io.Reader{
		"reader at": func() io.Reader { return bytes.NewReader(data) },
		// Hide io.ReaderAt to exercise the buffered path.
		"plain reader": func() io.Reader { return io.MultiReader(bytes.NewReader(data)) },
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fs, srv := newFakeStorage(t)
			fs.failOnce[2] = true
			c, err := New(Config{Endpoint: srv.URL + "/api/v1", Token: "tok", DefaultBucket: "uploads", HTTPClient: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			var lastLoaded int64
			res, err := c.MultipartUpload(context.Background(), body(), int64(len(data)), MultipartUploadOptions{
				UploadOptions: UploadOptions{Key: "big.bin", OnProgress: func(loaded, _ int64) { lastLoaded = loaded }},
				Concurrency:   3,
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.ETag != "final" {
				t.Fatalf("res = %#v", res)
			}
			if len(fs.completed) != 3 {
				t.Fatalf("completed parts = %#v", fs.completed)
			}
			for i, p := range fs.completed {
				if p.PartNumber != i+1 || p.ETag != "etag-"+strconv.Itoa(i+1) {
					t.Fatalf("part %d = %#v", i, p)
				}
			}
			if !bytes.Equal(fs.assembled(), data) {
				t.Fatal("assembled object does not match input")
			}
			if lastLoaded != int64(len(data)) {
				t.Fatalf("progress loaded = %d", lastLoaded)
			}
			if fs.createSize == nil || *fs.createSize != int64(len(data)) {
				t.Fatalf("create size = %v", fs.createSize)
			}
		})
	}
}

func TestMultipartUploadHonorsReaderOffset(t *testing.T) {
	t.Parallel()
	fs, srv := newFakeStorage(t)
	c, err := New(Config{Endpoint: srv.URL + "/api/v1", DefaultBucket: "uploads", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader([]byte("HEADERpayload"))
	if _, err := r.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MultipartUpload(context.Background(), r, 7, MultipartUploadOptions{UploadOptions: UploadOptions{Key: "x"}}); err != nil {
		t.Fatal(err)
	}
	if string(fs.assembled()) != "payload" {
		t.Fatalf("assembled = %q", fs.assembled())
	}
}

func TestMultipartUploadAbortsOnFailure(t *testing.T) {
	t.Parallel()
	fs, srv := newFakeStorage(t)
	c, err := New(Config{Endpoint: srv.URL + "/api/v1", DefaultBucket: "uploads", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	fs.failOnce[1] = true
	_, err = c.MultipartUpload(context.Background(), bytes.NewReader([]byte("abc")), 3, MultipartUploadOptions{
		UploadOptions: UploadOptions{Key: "x"},
		MaxRetries:    -1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !fs.aborted {
		t.Fatal("expected multipart abort")
	}
}

func TestChoosePartSize(t *testing.T) {
	const mib = 1024 * 1024
	cases := []struct {
		size, requested, want int64
		wantErr               bool
	}{
		{size: 1, requested: 0, want: minPartSize},
		{size: 100 * mib, requested: 8 * mib, want: 8 * mib},
		{size: 100 * 1024 * mib, requested: 0, want: 11 * mib},
		{size: 1, requested: maxPartSize + 1, wantErr: true},
		{size: maxPartSize*maxParts + 1, requested: 0, wantErr: true},
	}
	for _, tc := range cases {
		got, err := choosePartSize(tc.size, tc.requested)
		if (err != nil) != tc.wantErr {
			t.Fatalf("choosePartSize(%d,%d) err=%v", tc.size, tc.requested, err)
		}
		if !tc.wantErr {
			if got != tc.want {
				t.Fatalf("choosePartSize(%d,%d) = %d, want %d", tc.size, tc.requested, got, tc.want)
			}
			if (tc.size+got-1)/got > maxParts {
				t.Fatalf("part count exceeds limit for size %d", tc.size)
			}
		}
	}
}

func TestHeadExists(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body.Key {
		case "yes":
			_ = json.NewEncoder(w).Encode(map[string]any{"size": 1})
		case "no":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found","message":"not found"}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden","message":"not found in allowed prefixes"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c, _ := New(Config{Endpoint: srv.URL, DefaultBucket: "uploads", HTTPClient: srv.Client()})
	if ok, err := c.HeadExists(context.Background(), "yes", nil); !ok || err != nil {
		t.Fatalf("yes: %v %v", ok, err)
	}
	if ok, err := c.HeadExists(context.Background(), "no", nil); ok || err != nil {
		t.Fatalf("no: %v %v", ok, err)
	}
	if _, err := c.HeadExists(context.Background(), "denied", nil); err == nil {
		t.Fatal("403 must be returned as an error even if the message says 'not found'")
	}
}

func TestSyncPrefix(t *testing.T) {
	t.Parallel()
	objects := map[string]ListEntry{
		"src/a":    {Key: "src/a", Size: 1, ETag: "ea"},
		"src/b":    {Key: "src/b", Size: 2, ETag: "eb"},
		"dst/a":    {Key: "dst/a", Size: 1, ETag: "ea"},
		"dst/b":    {Key: "dst/b", Size: 2, ETag: "old"},
		"dst/gone": {Key: "dst/gone", Size: 3, ETag: "eg"},
	}
	var mu sync.Mutex
	var copies, deletes []string
	mux := http.NewServeMux()
	mux.HandleFunc("/object/list", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prefix string `json:"prefix"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		entries := []ListEntry{}
		for k, e := range objects {
			if strings.HasPrefix(k, body.Prefix) {
				entries = append(entries, e)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": entries})
	})
	mux.HandleFunc("/object/copy", func(w http.ResponseWriter, r *http.Request) {
		var body CopyOptions
		_ = json.NewDecoder(r.Body).Decode(&struct {
			SrcKey  *string `json:"srcKey"`
			DestKey *string `json:"destKey"`
		}{&body.SrcKey, &body.DestKey})
		mu.Lock()
		copies = append(copies, body.DestKey)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/object/delete", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		deletes = append(deletes, body.Key)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := New(Config{Endpoint: srv.URL, DefaultBucket: "uploads", HTTPClient: srv.Client()})

	res, err := c.SyncPrefix(context.Background(), "src", "dst", &SyncOptions{DryRun: true, DeleteOrphans: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 0 || len(deletes) != 0 {
		t.Fatalf("dry run mutated: copies=%v deletes=%v", copies, deletes)
	}
	if len(res.Copied) != 1 || res.Copied[0] != "dst/b" || len(res.Skipped) != 1 || len(res.Deleted) != 1 {
		t.Fatalf("dry run result = %#v", res)
	}

	if _, err := c.SyncPrefix(context.Background(), "src", "dst", nil); err != nil {
		t.Fatal(err)
	}
	if len(copies) != 1 || copies[0] != "dst/b" || len(deletes) != 0 {
		t.Fatalf("copies=%v deletes=%v", copies, deletes)
	}

	if _, err := c.SyncPrefix(context.Background(), "src", "src/nested", nil); err == nil {
		t.Fatal("overlapping prefixes must be rejected")
	}
}
