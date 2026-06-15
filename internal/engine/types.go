package engine

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrPreconditionFailed = errors.New("precondition failed")

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

// ListPageV2 is a cursor-based object listing page.
type ListPageV2 struct {
	Objects    []ObjectInfo
	Folders    []string
	NextCursor string
	HasMore    bool
}

// Engine is a physical storage backend (S3-compatible, etc.).
type Engine interface {
	Name() string
	CreateBucket(ctx context.Context, name string, meta BucketMeta) error
	DeleteBucket(ctx context.Context, name string) error
	ListBuckets(ctx context.Context) ([]string, error)
	GetBucketMeta(ctx context.Context, name string) (BucketMeta, string, error)
	SetBucketMeta(ctx context.Context, name string, meta BucketMeta, ifMatch string) error
	HeadBucket(ctx context.Context, name string) error
	EmptyBucket(ctx context.Context, name string) error

	PutObject(ctx context.Context, bucket, key, contentType, cacheControl string, body io.Reader, metadata map[string]string) error
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error)
	HeadObject(ctx context.Context, bucket, key string) (ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	DeleteObjects(ctx context.Context, bucket string, keys []string) error
	ListObjects(ctx context.Context, bucket, prefix string, limit, offset int) ([]ObjectInfo, error)
	ListObjectsV2(ctx context.Context, bucket, prefix string, limit int, cursor string, withDelimiter bool) (ListPageV2, error)
	CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error

	PresignGet(ctx context.Context, bucket, key string, expires time.Duration, downloadFilename string) (string, error)
	PresignPut(ctx context.Context, bucket, key, contentType string, expires time.Duration) (string, error)
}
