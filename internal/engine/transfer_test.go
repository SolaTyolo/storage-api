package engine

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type memEngine struct {
	name    string
	objects map[string][]byte
	types   map[string]string
}

func (m *memEngine) Name() string { return m.name }
func (m *memEngine) CreateBucket(context.Context, string, BucketMeta) error { return nil }
func (m *memEngine) DeleteBucket(context.Context, string) error             { return nil }
func (m *memEngine) ListBuckets(context.Context) ([]string, error)          { return nil, nil }
func (m *memEngine) GetBucketMeta(context.Context, string) (BucketMeta, string, error) {
	return BucketMeta{}, "", nil
}
func (m *memEngine) SetBucketMeta(context.Context, string, BucketMeta, string) error { return nil }
func (m *memEngine) HeadBucket(context.Context, string) error                        { return nil }
func (m *memEngine) EmptyBucket(context.Context, string) error                       { return nil }
func (m *memEngine) PutObject(_ context.Context, _, key, contentType, _ string, body io.Reader, _ map[string]string) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.objects[key] = b
	m.types[key] = contentType
	return nil
}
func (m *memEngine) GetObject(_ context.Context, _, key string) (io.ReadCloser, ObjectInfo, error) {
	b, ok := m.objects[key]
	if !ok {
		return nil, ObjectInfo{}, ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(b))), ObjectInfo{Path: key, Size: int64(len(b)), ContentType: m.types[key]}, nil
}
func (m *memEngine) HeadObject(_ context.Context, _, key string) (ObjectInfo, error) {
	b, ok := m.objects[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{Path: key, Size: int64(len(b)), ContentType: m.types[key]}, nil
}
func (m *memEngine) DeleteObject(context.Context, string, string) error    { return nil }
func (m *memEngine) DeleteObjects(context.Context, string, []string) error { return nil }
func (m *memEngine) ListObjects(context.Context, string, string, int, int) ([]ObjectInfo, error) {
	return nil, nil
}
func (m *memEngine) ListObjectsV2(context.Context, string, string, int, string, bool) (ListPageV2, error) {
	return ListPageV2{}, nil
}
func (m *memEngine) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	b, ok := m.objects[srcKey]
	if !ok {
		return ErrNotFound
	}
	m.objects[dstKey] = append([]byte(nil), b...)
	m.types[dstKey] = m.types[srcKey]
	return nil
}
func (m *memEngine) PresignGet(context.Context, string, string, time.Duration, string) (string, error) {
	return "", nil
}
func (m *memEngine) PresignPut(context.Context, string, string, string, time.Duration) (string, error) {
	return "", nil
}

func TestTransferObjectCrossEngine(t *testing.T) {
	src := &memEngine{name: "a", objects: map[string][]byte{"x": []byte("data")}, types: map[string]string{"x": "text/plain"}}
	dst := &memEngine{name: "b", objects: map[string][]byte{}, types: map[string]string{}}
	if err := TransferObject(context.Background(), src, dst, "b1", "x", "b2", "y"); err != nil {
		t.Fatal(err)
	}
	if string(dst.objects["y"]) != "data" {
		t.Fatalf("got %q", dst.objects["y"])
	}
}
