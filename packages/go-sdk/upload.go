package objsdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// S3 multipart limits.
const (
	minPartSize int64 = 5 * 1024 * 1024
	maxPartSize int64 = 5 * 1024 * 1024 * 1024
	maxParts    int64 = 10000

	defaultConcurrency = 4
	defaultMaxRetries  = 3
	retryBaseDelay     = 200 * time.Millisecond
	retryMaxDelay      = 5 * time.Second
)

// httpStatusError is returned by putObject for non-2xx storage responses.
type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("PUT failed: %d %s", e.Status, e.Body)
}

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
		Size:        &size,
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

// choosePartSize returns a part size that keeps the upload within S3 limits.
func choosePartSize(size, requested int64) (int64, error) {
	partSize := max(requested, minPartSize)
	if partSize > maxPartSize {
		return 0, fmt.Errorf("partSize must be <= %d", maxPartSize)
	}
	if size > partSize*maxParts {
		const mib = 1024 * 1024
		partSize = ((size+maxParts-1)/maxParts + mib - 1) / mib * mib
	}
	if partSize > maxPartSize {
		return 0, fmt.Errorf("object size %d exceeds the multipart limit", size)
	}
	return partSize, nil
}

type partJob struct {
	number int
	url    string
	size   int64
	// open returns a fresh reader per attempt; the transport may still hold the
	// previous attempt's body after Do returns.
	open func() io.Reader
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
	partSize, err := choosePartSize(size, opts.PartSize)
	if err != nil {
		return nil, err
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	} else if maxRetries < 0 {
		maxRetries = 0
	}
	totalParts := int(max(1, (size+partSize-1)/partSize))

	// Random access lets parts be read in parallel and rewound for retries without buffering.
	readerAt, _ := body.(io.ReaderAt)
	var baseOffset int64
	if readerAt != nil {
		if s, ok := body.(io.Seeker); ok {
			if baseOffset, err = s.Seek(0, io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("seek body: %w", err)
			}
		}
	}

	created, err := c.api.multipartCreate(ctx, bucketKeyExpire{
		Bucket:      bucket,
		Key:         opts.Key,
		ContentType: contentType,
		Meta:        opts.Meta,
		ExpireSec:   opts.ExpireSec,
		Size:        &size,
	})
	if err != nil {
		return nil, err
	}
	uploadID := created.UploadID
	abort := func() {
		_ = c.api.multipartAbort(context.WithoutCancel(ctx), bucket, opts.Key, uploadID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		firstErr error
		uploaded int64
		parts    = make([]CompletePart, totalParts)
		wg       sync.WaitGroup
		jobs     = make(chan partJob)
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}

	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				etag, err := c.uploadPartWithRetry(runCtx, job, maxRetries)
				if err != nil {
					fail(fmt.Errorf("part %d: %w", job.number, err))
					continue
				}
				mu.Lock()
				parts[job.number-1] = CompletePart{PartNumber: job.number, ETag: etag}
				uploaded += job.size
				if opts.OnProgress != nil {
					opts.OnProgress(uploaded, size)
				}
				mu.Unlock()
			}
		}()
	}

	// Sign lazily in small batches so URLs do not expire before their part is sent.
	signBatch := concurrency * 2
produce:
	for first := 1; first <= totalParts; first += signBatch {
		last := min(first+signBatch-1, totalParts)
		numbers := make([]int, 0, last-first+1)
		for n := first; n <= last; n++ {
			numbers = append(numbers, n)
		}
		signed, err := c.api.multipartPresignParts(runCtx, bucket, opts.Key, uploadID, numbers, opts.ExpireSec)
		if err != nil {
			fail(err)
			break
		}
		for _, sp := range signed.Parts {
			offset := int64(sp.PartNumber-1) * partSize
			partLen := min(partSize, size-offset)
			var open func() io.Reader
			if readerAt != nil {
				start := baseOffset + offset
				open = func() io.Reader { return io.NewSectionReader(readerAt, start, partLen) }
			} else {
				buf := make([]byte, partLen)
				if _, err := io.ReadFull(body, buf); err != nil {
					fail(fmt.Errorf("read part %d: %w", sp.PartNumber, err))
					break produce
				}
				open = func() io.Reader { return bytes.NewReader(buf) }
			}
			select {
			case jobs <- partJob{number: sp.PartNumber, url: sp.URL, size: partLen, open: open}:
			case <-runCtx.Done():
				break produce
			}
		}
	}
	close(jobs)
	wg.Wait()

	if firstErr == nil && ctx.Err() != nil {
		firstErr = ctx.Err()
	}
	if firstErr != nil {
		abort()
		return nil, firstErr
	}

	completed, err := c.api.multipartComplete(ctx, bucket, opts.Key, uploadID, parts)
	if err != nil {
		abort()
		return nil, err
	}

	return c.buildResult(ctx, bucket, opts.Key, "", completed.ETag, size, contentType, opts.Meta)
}

func (c *Client) uploadPartWithRetry(ctx context.Context, job partJob, maxRetries int) (string, error) {
	for attempt := 0; ; attempt++ {
		etag, err := c.putObject(ctx, job.url, job.open(), job.size, "", nil, nil)
		if err == nil {
			if etag == "" {
				return "", fmt.Errorf("missing ETag (expose ETag in storage CORS)")
			}
			return etag, nil
		}
		if attempt >= maxRetries || !retryable(ctx, err) {
			return "", err
		}
		delay := min(retryBaseDelay<<attempt, retryMaxDelay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.Status >= 500 || se.Status == http.StatusTooManyRequests || se.Status == http.StatusRequestTimeout
	}
	return true
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
	if size == 0 {
		// A non-nil body with ContentLength 0 is sent chunked, which breaks a signed Content-Length.
		req.Body = http.NoBody
	}
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
		return "", &httpStatusError{Status: res.StatusCode, Body: string(b)}
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
	r      io.Reader
	total  int64
	loaded int64
	fn     ProgressFunc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.loaded += int64(n)
		p.fn(p.loaded, p.total)
	}
	return n, err
}
