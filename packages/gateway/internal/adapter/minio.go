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
	client         *minio.Client
	core           *minio.Core
	cdnDomain      string
	publicEndpoint string // scheme://host[:port] for browser-facing presigned URLs
}

// NewMinioAdapter builds an adapter for one bucket config.
func NewMinioAdapter(cfg config.BucketConfig) (*MinioAdapter, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("minio client for %q: %w", cfg.Name, err)
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("minio core for %q: %w", cfg.Name, err)
	}
	return &MinioAdapter{
		client:         client,
		core:           core,
		cdnDomain:      strings.TrimRight(cfg.CDNDomain, "/"),
		publicEndpoint: strings.TrimRight(cfg.PublicEndpoint, "/"),
	}, nil
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
	u, err := a.client.PresignHeader(ctx, http.MethodPut, bucket, key, expiry, nil, headers)
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return a.rewritePublicURL(u.String()), nil
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
	u, err := a.client.Presign(ctx, http.MethodPut, bucket, key, expiry, url.Values{
		"uploadId":   []string{uploadID},
		"partNumber": []string{fmt.Sprintf("%d", partNumber)},
	})
	if err != nil {
		return "", fmt.Errorf("presign upload part: %w", err)
	}
	return a.rewritePublicURL(u.String()), nil
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
	u, err := a.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return a.rewritePublicURL(u.String()), nil
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

func (a *MinioAdapter) CDNURL(bucket, key string) string {
	_ = bucket
	base := a.cdnDomain
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimPrefix(key, "/")
}

// rewritePublicURL replaces the internal S3 host with PublicEndpoint for browsers.
func (a *MinioAdapter) rewritePublicURL(raw string) string {
	if a.publicEndpoint == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	pub, err := url.Parse(a.publicEndpoint)
	if err != nil || pub.Host == "" {
		return raw
	}
	u.Scheme = pub.Scheme
	u.Host = pub.Host
	return u.String()
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
