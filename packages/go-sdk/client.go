package objsdk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the Go SDK for obj-gateway (presign control plane + direct object I/O).
type Client struct {
	api           *gatewayAPI
	http          HTTPDoer
	defaultBucket string
}

// New builds a Client from Config.
func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Client{
		api:           newGatewayAPI(endpoint, cfg.Token, httpClient),
		http:          httpClient,
		defaultBucket: cfg.DefaultBucket,
	}, nil
}

func (c *Client) resolveBucket(bucket string) (string, error) {
	b := bucket
	if b == "" {
		b = c.defaultBucket
	}
	if b == "" {
		return "", fmt.Errorf("bucket is required (pass options.Bucket or Config.DefaultBucket)")
	}
	return b, nil
}

// PresignPut returns a short-lived PUT URL (and CDN URL) without uploading bytes.
// Use when the caller (e.g. a browser) will PUT the object itself.
func (c *Client) PresignPut(ctx context.Context, opts UploadOptions) (*PresignPutResponse, error) {
	if strings.TrimSpace(opts.Key) == "" {
		return nil, fmt.Errorf("key is required")
	}
	bucket, err := c.resolveBucket(opts.Bucket)
	if err != nil {
		return nil, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return c.api.presignPut(ctx, bucketKeyExpire{
		Bucket:      bucket,
		Key:         opts.Key,
		ContentType: contentType,
		Meta:        opts.Meta,
		ExpireSec:   opts.ExpireSec,
	})
}

// Upload uploads body via gateway presign + PUT directly to object storage.
// size must be the exact Content-Length of body (>= 0).
func (c *Client) Upload(ctx context.Context, body io.Reader, size int64, opts UploadOptions) (*UploadResult, error) {
	return c.upload(ctx, body, size, opts)
}

// UploadBytes uploads data via gateway presign + PUT.
func (c *Client) UploadBytes(ctx context.Context, data []byte, opts UploadOptions) (*UploadResult, error) {
	return c.upload(ctx, bytes.NewReader(data), int64(len(data)), opts)
}

// MultipartUpload uploads a large object via S3 multipart (control plane on gateway).
// size must be the exact length of body (>= 0).
func (c *Client) MultipartUpload(ctx context.Context, body io.Reader, size int64, opts MultipartUploadOptions) (*UploadResult, error) {
	return c.multipartUpload(ctx, body, size, opts)
}

// DownloadURL returns a short-lived GET URL for a private object.
func (c *Client) DownloadURL(ctx context.Context, key string, opts *DownloadOptions) (string, error) {
	o := DownloadOptions{}
	if opts != nil {
		o = *opts
	}
	bucket, err := c.resolveBucket(o.Bucket)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("key is required")
	}
	res, err := c.api.presignGet(ctx, bucket, key, o.ExpireSec)
	if err != nil {
		return "", err
	}
	return res.DownloadURL, nil
}

// Download fetches object bytes via a presigned GET URL.
// The caller must close the returned body.
func (c *Client) Download(ctx context.Context, key string, opts *DownloadOptions) (*http.Response, error) {
	url, err := c.DownloadURL(ctx, key, opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("GET failed: %d %s", res.StatusCode, string(b))
	}
	return res, nil
}

// Head reads object metadata via the gateway.
func (c *Client) Head(ctx context.Context, key string, opts *HeadOptions) (*ObjectHeadResponse, error) {
	o := HeadOptions{}
	if opts != nil {
		o = *opts
	}
	bucket, err := c.resolveBucket(o.Bucket)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("key is required")
	}
	return c.api.objectHead(ctx, bucket, key)
}

// DownloadOptions configures Download / DownloadURL.
type DownloadOptions struct {
	Bucket    string
	ExpireSec int
}

// HeadOptions configures Head.
type HeadOptions struct {
	Bucket string
}
