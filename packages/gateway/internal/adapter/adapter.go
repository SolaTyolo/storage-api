package adapter

import (
	"context"
	"time"
)

// CompletePart is one part in a multipart complete request.
type CompletePart struct {
	PartNumber int
	ETag       string
}

// ObjectInfo is object metadata returned by HeadObject.
type ObjectInfo struct {
	Size        int64
	ContentType string
	ETag        string
	Meta        map[string]string
}

// ListEntry is one object or common-prefix from ListObjects.
type ListEntry struct {
	Key          string
	Size         int64
	LastModified time.Time
	IsDir        bool
}

// ListResult is a page of ListObjects results.
type ListResult struct {
	Entries            []ListEntry
	IsTruncated        bool
	NextContinuationToken string
}

// ObjectAdapter isolates S3-compatible vendor differences.
type ObjectAdapter interface {
	PresignPut(ctx context.Context, bucket, key, contentType string, meta map[string]string, expiry time.Duration) (string, error)
	CreateMultipart(ctx context.Context, bucket, key, contentType string, meta map[string]string) (uploadID string, err error)
	PresignUploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, expiry time.Duration) (string, error)
	CompleteMultipart(ctx context.Context, bucket, key, uploadID string, parts []CompletePart) (etag string, err error)
	AbortMultipart(ctx context.Context, bucket, key, uploadID string) error
	PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
	HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	ListObjects(ctx context.Context, bucket, prefix, delimiter, continuationToken string, maxKeys int) (*ListResult, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	CopyObject(ctx context.Context, bucket, srcKey, destKey string) error
	CDNURL(bucket, key string) string
}
