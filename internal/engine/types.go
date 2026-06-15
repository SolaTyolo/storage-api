package engine

import (
	"context"
	"io"
	"time"
)

const bucketMetaKey = ".__storage/bucket.json"

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Path         string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	Metadata     map[string]string
}

// BucketMeta is persisted as JSON in each physical bucket.
type BucketMeta struct {
	Public           bool     `json:"public"`
	FileSizeLimit    *int64   `json:"file_size_limit,omitempty"`
	AllowedMimeTypes []string `json:"allowed_mime_types,omitempty"`
}

// ResolvedBucket maps a Supabase bucket id to a physical engine + bucket.
type ResolvedBucket struct {
	Engine     string
	Bucket     string
	DisplayID  string
	Public     bool
	FileSizeLimit    *int64
	AllowedMimeTypes []string
}

// Engine is a physical storage backend (S3-compatible, etc.).
type Engine interface {
	Name() string
	CreateBucket(ctx context.Context, name string, meta BucketMeta) error
	DeleteBucket(ctx context.Context, name string) error
	ListBuckets(ctx context.Context) ([]string, error)
	GetBucketMeta(ctx context.Context, name string) (BucketMeta, error)
	SetBucketMeta(ctx context.Context, name string, meta BucketMeta) error
	HeadBucket(ctx context.Context, name string) error
	EmptyBucket(ctx context.Context, name string) error

	PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader, metadata map[string]string) error
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error)
	HeadObject(ctx context.Context, bucket, key string) (ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	DeleteObjects(ctx context.Context, bucket string, keys []string) error
	ListObjects(ctx context.Context, bucket, prefix string, limit, offset int) ([]ObjectInfo, error)
	CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error

	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	PresignPut(ctx context.Context, bucket, key, contentType string, expires time.Duration) (string, error)
}
