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

// List returns one page of objects/folders under prefix.
// Pass Delimiter "/" for a single-level (folder) listing.
func (c *Client) List(ctx context.Context, opts ListOptions) (*ListObjectsResponse, error) {
	bucket, err := c.resolveBucket(opts.Bucket)
	if err != nil {
		return nil, err
	}
	opts.Bucket = bucket
	return c.api.objectList(ctx, opts)
}

// ListAllKeys recursively lists object keys under prefix (no delimiter).
func (c *Client) ListAllKeys(ctx context.Context, prefix string, opts *ListOptions) ([]string, error) {
	o := ListOptions{}
	if opts != nil {
		o = *opts
	}
	o.Prefix = prefix
	o.Delimiter = ""
	bucket, err := c.resolveBucket(o.Bucket)
	if err != nil {
		return nil, err
	}
	o.Bucket = bucket
	var keys []string
	for {
		page, err := c.api.objectList(ctx, o)
		if err != nil {
			return nil, err
		}
		for _, e := range page.Entries {
			if e.IsDir || e.Key == "" || strings.HasSuffix(e.Key, "/") {
				continue
			}
			keys = append(keys, e.Key)
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			break
		}
		o.ContinuationToken = page.NextContinuationToken
	}
	return keys, nil
}

// Delete removes an object key.
func (c *Client) Delete(ctx context.Context, key string, opts *HeadOptions) error {
	o := HeadOptions{}
	if opts != nil {
		o = *opts
	}
	bucket, err := c.resolveBucket(o.Bucket)
	if err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("key is required")
	}
	return c.api.objectDelete(ctx, bucket, key)
}

// Copy performs a server-side copy within a bucket.
func (c *Client) Copy(ctx context.Context, opts CopyOptions) error {
	bucket, err := c.resolveBucket(opts.Bucket)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.SrcKey) == "" || strings.TrimSpace(opts.DestKey) == "" {
		return fmt.Errorf("srcKey and destKey are required")
	}
	return c.api.objectCopy(ctx, bucket, opts.SrcKey, opts.DestKey)
}

// Mkdir creates a zero-byte folder marker (key ends with "/").
func (c *Client) Mkdir(ctx context.Context, dir string, opts *UploadOptions) error {
	o := UploadOptions{}
	if opts != nil {
		o = *opts
	}
	dir = strings.TrimSpace(dir)
	dir = strings.ReplaceAll(dir, "\\", "/")
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return fmt.Errorf("empty folder path")
	}
	o.Key = dir + "/"
	o.ContentType = "application/x-directory"
	_, err := c.UploadBytes(ctx, nil, o)
	return err
}

// GetText downloads a small object as UTF-8 text (max 1 MiB).
func (c *Client) GetText(ctx context.Context, key string, opts *DownloadOptions) (string, error) {
	res, err := c.Download(ctx, key, opts)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PutText uploads a small UTF-8 text object via presigned PUT.
func (c *Client) PutText(ctx context.Context, key, body string, opts *UploadOptions) error {
	o := UploadOptions{}
	if opts != nil {
		o = *opts
	}
	o.Key = key
	if o.ContentType == "" {
		o.ContentType = "text/plain; charset=utf-8"
	}
	_, err := c.UploadBytes(ctx, []byte(body), o)
	return err
}

// HeadExists reports whether an object exists.
func (c *Client) HeadExists(ctx context.Context, key string, opts *HeadOptions) (bool, error) {
	_, err := c.Head(ctx, key, opts)
	if err == nil {
		return true, nil
	}
	if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
		return false, nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") || strings.Contains(msg, "nosuchkey") {
		return false, nil
	}
	return false, err
}

// SyncPrefix copies all objects from srcPrefix to destPrefix and deletes
// destination keys that are not present in the source.
func (c *Client) SyncPrefix(ctx context.Context, srcPrefix, destPrefix string, opts *ListOptions) error {
	srcPrefix = normalizePrefix(srcPrefix)
	destPrefix = normalizePrefix(destPrefix)
	if srcPrefix == "" || destPrefix == "" {
		return fmt.Errorf("srcPrefix and destPrefix are required")
	}
	bucket := ""
	if opts != nil {
		bucket = opts.Bucket
	}
	srcKeys, err := c.ListAllKeys(ctx, srcPrefix, opts)
	if err != nil {
		return err
	}
	if len(srcKeys) == 0 {
		return fmt.Errorf("source prefix empty: %s", srcPrefix)
	}
	destKeys, err := c.ListAllKeys(ctx, destPrefix, opts)
	if err != nil {
		return err
	}
	srcRel := map[string]string{}
	for _, k := range srcKeys {
		rel := strings.TrimPrefix(k, srcPrefix)
		if rel == "" {
			continue
		}
		srcRel[rel] = k
	}
	for rel, srcKey := range srcRel {
		destKey := destPrefix + rel
		if err := c.Copy(ctx, CopyOptions{Bucket: bucket, SrcKey: srcKey, DestKey: destKey}); err != nil {
			return fmt.Errorf("copy %s → %s: %w", srcKey, destKey, err)
		}
	}
	headOpts := &HeadOptions{Bucket: bucket}
	for _, k := range destKeys {
		rel := strings.TrimPrefix(k, destPrefix)
		if rel == "" {
			continue
		}
		if _, ok := srcRel[rel]; ok {
			continue
		}
		if err := c.Delete(ctx, k, headOpts); err != nil {
			return fmt.Errorf("delete orphan %s: %w", k, err)
		}
	}
	return nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.ReplaceAll(prefix, "\\", "/")
	prefix = strings.TrimLeft(prefix, "/")
	if prefix == "" {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
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
