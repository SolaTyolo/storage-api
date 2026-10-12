package objsdk

import "net/http"

// Config configures the obj-gateway client.
type Config struct {
	// Endpoint is the gateway base URL, e.g. http://localhost:8080/api/v1
	Endpoint string
	// Token is the JWT Bearer token (service_role recommended for backends).
	Token string
	// DefaultBucket is used when a call omits Bucket.
	DefaultBucket string
	// HTTPClient overrides the default http.Client (optional).
	HTTPClient HTTPDoer
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
	// PartSize in bytes (minimum / default 5 MiB).
	PartSize int64
}

// ProgressFunc reports upload progress.
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
