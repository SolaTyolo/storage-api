package objsdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultPartSize int64 = 5 * 1024 * 1024

func (c *Client) upload(ctx context.Context, body io.Reader, size int64, opts UploadOptions) (*UploadResult, error) {
	if strings.TrimSpace(opts.Key) == "" {
		return nil, fmt.Errorf("key is required")
	}
	if size < 0 {
		return nil, fmt.Errorf("size must be >= 0")
	}
	bucket, err := c.resolveBucket(opts.Bucket)
	if err != nil {
		return nil, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	signed, err := c.api.presignPut(ctx, bucketKeyExpire{
		Bucket:      bucket,
		Key:         opts.Key,
		ContentType: contentType,
		Meta:        opts.Meta,
		ExpireSec:   opts.ExpireSec,
	})
	if err != nil {
		return nil, err
	}

	etag, err := c.putObject(ctx, signed.UploadURL, body, size, contentType, opts.Meta, opts.OnProgress)
	if err != nil {
		return nil, err
	}

	return c.buildResult(ctx, bucket, opts.Key, signed.CDNURL, etag, size, contentType, opts.Meta)
}

func (c *Client) multipartUpload(ctx context.Context, body io.Reader, size int64, opts MultipartUploadOptions) (*UploadResult, error) {
	if strings.TrimSpace(opts.Key) == "" {
		return nil, fmt.Errorf("key is required")
	}
	if size < 0 {
		return nil, fmt.Errorf("size must be >= 0")
	}
	bucket, err := c.resolveBucket(opts.Bucket)
	if err != nil {
		return nil, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	partSize := opts.PartSize
	if partSize < defaultPartSize {
		partSize = defaultPartSize
	}

	created, err := c.api.multipartCreate(ctx, bucketKeyExpire{
		Bucket:      bucket,
		Key:         opts.Key,
		ContentType: contentType,
		Meta:        opts.Meta,
		ExpireSec:   opts.ExpireSec,
	})
	if err != nil {
		return nil, err
	}
	uploadID := created.UploadID

	parts := make([]CompletePart, 0)
	var uploaded int64

	abort := func() {
		_ = c.api.multipartAbort(context.WithoutCancel(ctx), bucket, opts.Key, uploadID)
	}

	totalParts := int((size + partSize - 1) / partSize)
	if totalParts < 1 {
		totalParts = 1
	}

	for i := 0; i < totalParts; i++ {
		partNumber := i + 1
		start := int64(i) * partSize
		remain := size - start
		thisSize := partSize
		if remain < thisSize {
			thisSize = remain
		}
		if size == 0 {
			thisSize = 0
		}

		partURL, err := c.api.multipartPresignPart(ctx, bucket, opts.Key, uploadID, partNumber, opts.ExpireSec)
		if err != nil {
			abort()
			return nil, err
		}

		partBody := io.LimitReader(body, thisSize)
		etag, err := c.putObject(ctx, partURL.PartUploadURL, partBody, thisSize, "", nil, nil)
		if err != nil {
			abort()
			return nil, err
		}
		if etag == "" {
			abort()
			return nil, fmt.Errorf("missing ETag for part %d", partNumber)
		}
		parts = append(parts, CompletePart{PartNumber: partNumber, ETag: etag})
		uploaded += thisSize
		if opts.OnProgress != nil {
			opts.OnProgress(uploaded, size)
		}
	}

	completed, err := c.api.multipartComplete(ctx, bucket, opts.Key, uploadID, parts)
	if err != nil {
		abort()
		return nil, err
	}

	return c.buildResult(ctx, bucket, opts.Key, "", completed.ETag, size, contentType, opts.Meta)
}

func (c *Client) buildResult(ctx context.Context, bucket, key, cdnURL, etag string, size int64, contentType string, meta Meta) (*UploadResult, error) {
	head, err := c.api.objectHead(ctx, bucket, key)
	if err != nil {
		if meta == nil {
			meta = Meta{}
		}
		return &UploadResult{
			Bucket:      bucket,
			Key:         key,
			CDNURL:      cdnURL,
			ETag:        etag,
			Size:        size,
			ContentType: contentType,
			Meta:        meta,
		}, nil
	}
	outCDN := cdnURL
	if outCDN == "" {
		outCDN = head.CDNURL
	}
	outETag := head.ETag
	if outETag == "" {
		outETag = etag
	}
	outMeta := head.Meta
	if outMeta == nil {
		outMeta = meta
	}
	if outMeta == nil {
		outMeta = Meta{}
	}
	outSize := head.Size
	if outSize == 0 && size > 0 {
		outSize = size
	}
	outCT := head.ContentType
	if outCT == "" {
		outCT = contentType
	}
	return &UploadResult{
		Bucket:      bucket,
		Key:         key,
		CDNURL:      outCDN,
		ETag:        outETag,
		Size:        outSize,
		ContentType: outCT,
		Meta:        outMeta,
	}, nil
}

func (c *Client) putObject(ctx context.Context, url string, body io.Reader, size int64, contentType string, meta Meta, onProgress ProgressFunc) (string, error) {
	var reader io.Reader = body
	if onProgress != nil && size > 0 {
		reader = &progressReader{r: body, total: size, fn: onProgress}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, reader)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range metaHeaders(meta) {
		req.Header.Set(k, v)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("PUT failed: %d %s", res.StatusCode, string(b))
	}
	etag := strings.Trim(res.Header.Get("ETag"), `"`)
	return etag, nil
}

func metaHeaders(meta Meta) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		key := strings.ToLower(k)
		if !strings.HasPrefix(key, "x-amz-meta-") {
			key = "x-amz-meta-" + key
		}
		out[key] = v
	}
	return out
}

type progressReader struct {
	r       io.Reader
	total   int64
	loaded  int64
	fn      ProgressFunc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.loaded += int64(n)
		p.fn(p.loaded, p.total)
	}
	return n, err
}
