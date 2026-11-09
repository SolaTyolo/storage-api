package objsdk

import (
	"net/http"
	"time"
)

// Config configures the obj-gateway client.
type Config struct {
	// Endpoint is the gateway base URL, e.g. http://localhost:8080/api/v1
	Endpoint string
	// Token is the JWT Bearer token (service_role recommended for backends).
	Token string
	// DefaultBucket is used when a call omits Bucket.
	DefaultBucket string
	// HTTPClient overrides the default http.Client (optional). It is used for both
	// gateway calls and object transfers, so avoid a short http.Client.Timeout;
	// use APITimeout and the call context instead.
	HTTPClient HTTPDoer
	// APITimeout bounds each gateway control-plane call (default 30s).
	// Object PUT/GET transfers are bounded only by the caller's context.
	APITimeout time.Duration
}

// HTTPDoer is the subset of http.Client used by the SDK.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Meta is user metadata stored as x-amz-meta-* headers.
type Meta map[string]string

// UploadOptions configures a single-object upload.
type UploadOptions struct {
	Key         string
	Bucket      string
	ContentType string
	Meta        Meta
	ExpireSec   int
	OnProgress  ProgressFunc
}

// MultipartUploadOptions extends UploadOptions for multipart uploads.
type MultipartUploadOptions struct {
	UploadOptions
	// PartSize in bytes (minimum / default 5 MiB). It is raised automatically
	// when the object would otherwise need more than 10,000 parts.
	PartSize int64
	// Concurrency is the number of parts uploaded in parallel (default 4).
	Concurrency int
	// MaxRetries per part on network errors and 5xx/429 responses (default 3; negative disables).
	MaxRetries int
}

// ProgressFunc reports upload progress. It is never called concurrently.
type ProgressFunc func(loaded, total int64)

// UploadResult is returned after a successful upload.
type UploadResult struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	CDNURL      string `json:"cdnUrl"`
	ETag        string `json:"etag"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	Meta        Meta   `json:"meta"`
}

// PresignPutResponse is the gateway /presign/put response.
type PresignPutResponse struct {
	UploadURL string `json:"uploadUrl"`
	CDNURL    string `json:"cdnUrl"`
}

// PresignGetResponse is the gateway /presign/get response.
type PresignGetResponse struct {
	DownloadURL string `json:"downloadUrl"`
}

// MultipartCreateResponse is the gateway /multipart/create response.
type MultipartCreateResponse struct {
	UploadID string `json:"uploadId"`
}

// MultipartPresignPartResponse is the gateway /multipart/presign-part response.
type MultipartPresignPartResponse struct {
	PartUploadURL string `json:"partUploadUrl"`
}

// SignedPart is one entry of the gateway /multipart/presign-parts response.
type SignedPart struct {
	PartNumber int    `json:"partNumber"`
	URL        string `json:"url"`
}

// MultipartPresignPartsResponse is the gateway /multipart/presign-parts response.
type MultipartPresignPartsResponse struct {
	Parts []SignedPart `json:"parts"`
}

// MultipartCompleteResponse is the gateway /multipart/complete response.
type MultipartCompleteResponse struct {
	ETag string `json:"etag"`
}

// CompletePart is one part in a multipart complete request.
type CompletePart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

// ObjectHeadResponse is the gateway /object/head response.
type ObjectHeadResponse struct {
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	Meta        Meta   `json:"meta"`
	ETag        string `json:"etag"`
	CDNURL      string `json:"cdnUrl"`
}

// ListEntry is one object or folder from /object/list.
type ListEntry struct {
	Key          string `json:"key"`
	IsDir        bool   `json:"isDir"`
	Size         int64  `json:"size,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	CDNURL       string `json:"cdnUrl,omitempty"`
}

// ListObjectsResponse is the gateway /object/list response.
type ListObjectsResponse struct {
	Entries               []ListEntry `json:"entries"`
	IsTruncated           bool        `json:"isTruncated"`
	NextContinuationToken string      `json:"nextContinuationToken"`
}

// ListOptions configures List / ListAllKeys.
type ListOptions struct {
	Bucket            string
	Prefix            string
	Delimiter         string
	ContinuationToken string
	MaxKeys           int
}

// CopyOptions configures Copy.
type CopyOptions struct {
	Bucket  string
	SrcKey  string
	DestKey string
}

// SyncOptions configures SyncPrefix.
type SyncOptions struct {
	Bucket string
	// DryRun reports planned changes without copying or deleting.
	DryRun bool
	// DeleteOrphans removes destination keys that do not exist under the source prefix.
	DeleteOrphans bool
}

// SyncResult lists the destination keys affected by SyncPrefix.
type SyncResult struct {
	Copied  []string
	Skipped []string
	Deleted []string
}
