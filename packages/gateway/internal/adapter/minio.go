package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioAdapter implements ObjectAdapter via minio-go (S3-compatible).
type MinioAdapter struct {
	client         *minio.Client // internal endpoint (gateway → storage)
	core           *minio.Core
	presign        *minio.Client // public endpoint for SigV4 (browser host must match signature)
	cdnDomain      string
	publicEndpoint string
}

// NewMinioAdapter builds an adapter for one bucket config.
func NewMinioAdapter(cfg config.BucketConfig) (*MinioAdapter, error) {
	client, core, err := newMinioPair(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, cfg.UseSSL, cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("minio client for %q: %w", cfg.Name, err)
	}

	presignEndpoint := cfg.Endpoint
	presignSSL := cfg.UseSSL
	if pe := strings.TrimSpace(cfg.PublicEndpoint); pe != "" {
		presignEndpoint = pe
		if u, err := url.Parse(pe); err == nil && u.Scheme != "" {
			presignSSL = u.Scheme == "https"
		}
	}
	presign, _, err := newMinioPair(presignEndpoint, cfg.AccessKey, cfg.SecretKey, presignSSL, cfg.Region)
	if err != nil {
		return nil, fmt.Errorf("presign client for %q: %w", cfg.Name, err)
	}

	return &MinioAdapter{
		client:         client,
		core:           core,
		presign:        presign,
		cdnDomain:      strings.TrimRight(cfg.CDNDomain, "/"),
		publicEndpoint: strings.TrimRight(cfg.PublicEndpoint, "/"),
	}, nil
}

func newMinioPair(rawEndpoint, accessKey, secretKey string, secure bool, region string) (*minio.Client, *minio.Core, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(rawEndpoint, "https://"), "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, nil, err
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, nil, err
	}
	return client, core, nil
}

func (a *MinioAdapter) PresignPut(ctx context.Context, bucket, key, contentType string, meta map[string]string, expiry time.Duration) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	for k, v := range normalizeMeta(meta) {
		headers.Set("x-amz-meta-"+k, v)
	}
	// Sign with the browser-facing host so Host in SigV4 matches the request.
	u, err := a.presign.PresignHeader(ctx, http.MethodPut, bucket, key, expiry, nil, headers)
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return u.String(), nil
}

func (a *MinioAdapter) CreateMultipart(ctx context.Context, bucket, key, contentType string, meta map[string]string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	uploadID, err := a.core.NewMultipartUpload(ctx, bucket, key, minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: normalizeMeta(meta),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart: %w", err)
	}
	return uploadID, nil
}

func (a *MinioAdapter) PresignUploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, expiry time.Duration) (string, error) {
	u, err := a.presign.Presign(ctx, http.MethodPut, bucket, key, expiry, url.Values{
		"uploadId":   []string{uploadID},
		"partNumber": []string{fmt.Sprintf("%d", partNumber)},
	})
	if err != nil {
		return "", fmt.Errorf("presign upload part: %w", err)
	}
	return u.String(), nil
}

func (a *MinioAdapter) CompleteMultipart(ctx context.Context, bucket, key, uploadID string, parts []CompletePart) (string, error) {
	complete := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		complete = append(complete, minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       strings.Trim(p.ETag, `"`),
		})
	}
	info, err := a.core.CompleteMultipartUpload(ctx, bucket, key, uploadID, complete, minio.PutObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("complete multipart: %w", err)
	}
	return strings.Trim(info.ETag, `"`), nil
}

func (a *MinioAdapter) AbortMultipart(ctx context.Context, bucket, key, uploadID string) error {
	if err := a.core.AbortMultipartUpload(ctx, bucket, key, uploadID); err != nil {
		return fmt.Errorf("abort multipart: %w", err)
	}
	return nil
}

func (a *MinioAdapter) PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := a.presign.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return u.String(), nil
}

func (a *MinioAdapter) HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error) {
	info, err := a.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}
	meta := make(map[string]string, len(info.UserMetadata))
	for k, v := range info.UserMetadata {
		meta[strings.ToLower(k)] = v
	}
	return &ObjectInfo{
		Size:        info.Size,
		ContentType: info.ContentType,
		ETag:        strings.Trim(info.ETag, `"`),
		Meta:        meta,
	}, nil
}

func (a *MinioAdapter) ListObjects(ctx context.Context, bucket, prefix, delimiter, continuationToken string, maxKeys int) (*ListResult, error) {
	_ = ctx
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	res, err := a.core.ListObjectsV2(bucket, prefix, "", continuationToken, delimiter, maxKeys)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	out := &ListResult{
		IsTruncated:           res.IsTruncated,
		NextContinuationToken: res.NextContinuationToken,
	}
	for _, p := range res.CommonPrefixes {
		if p.Prefix == "" || p.Prefix == prefix {
			continue
		}
		out.Entries = append(out.Entries, ListEntry{Key: p.Prefix, IsDir: true})
	}
	for _, obj := range res.Contents {
		if obj.Key == "" || obj.Key == prefix || strings.HasSuffix(obj.Key, "/") {
			continue
		}
		out.Entries = append(out.Entries, ListEntry{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			IsDir:        false,
		})
	}
	return out, nil
}

func (a *MinioAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	if err := a.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (a *MinioAdapter) CopyObject(ctx context.Context, bucket, srcKey, destKey string) error {
	src := minio.CopySrcOptions{Bucket: bucket, Object: srcKey}
	dst := minio.CopyDestOptions{Bucket: bucket, Object: destKey}
	if _, err := a.client.CopyObject(ctx, dst, src); err != nil {
		return fmt.Errorf("copy object: %w", err)
	}
	return nil
}

func (a *MinioAdapter) CDNURL(bucket, key string) string {
	_ = bucket
	base := a.cdnDomain
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimPrefix(key, "/")
}

func normalizeMeta(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		k = strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-")
		out[k] = v
	}
	return out
}
