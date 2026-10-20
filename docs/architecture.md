# Obj-gateway architecture

## Core design principles

1. **Gateway does not receive file streams**: PUT / multipart binary goes from the browser directly to object storage; the gateway only issues presigned URLs.
2. **No separate metadata database**: custom business metadata lives in S3 `x-amz-meta-*` headers; size / content-type / etag come from `HeadObject`.
3. **Native S3 semantics**: API aligns with PutObject / MultipartUpload; resource id is `bucket + key`.
4. **No AK/SK on the frontend**: cloud credentials stay in the Go gateway; clients authenticate with JWT and request presigned URLs.
5. **Multi-vendor adapter**: unified `minio-go` client; domestic clouds use S3-compatible mode.
6. **Dev / prod parity**: RustFS + Nginx (dev CDN) + playground locally; switch bucket config to OSS/COS in production without frontend changes. Use `endpoint` for in-cluster API calls and `publicEndpoint` when **signing** browser URLs (SigV4 Host must match the browser host — do not rewrite after signing).

## Data flow

```
Frontend obj-gateway-sdk
    ↓① HTTP → gateway API with JWT (presign / multipart)
Go Gateway
    ├ JWT + bucket/key prefix authorization
    ├ Resolve minio-go client from bucket config
    ├ Issue S3 presigned URL (ContentType + metadata)
    ↓② Return uploadUrl / uploadId / …
Frontend SDK
    ↓③ PUT bytes straight to object storage (+ x-amz-meta-*)
Object storage → CDN (Nginx → MinIO in development)
Business backend (packages/go-sdk): same presign control plane + direct PUT/GET;
persist bucket + key; call /object/head when metadata is needed
```

## ObjectAdapter

```go
type ObjectAdapter interface {
	PresignPut(ctx context.Context, bucket, key, contentType string, meta map[string]string, expiry time.Duration) (string, error)
	CreateMultipart(ctx context.Context, bucket, key, contentType string, meta map[string]string) (uploadID string, err error)
	PresignUploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, expiry time.Duration) (string, error)
	CompleteMultipart(ctx context.Context, bucket, key, uploadID string, parts []CompletePart) (etag string, err error)
	AbortMultipart(ctx context.Context, bucket, key, uploadID string) error
	PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
	HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	CDNURL(bucket, key string) string
}
```

## Security

- JWT before issuing any signature; optional `bucket` + `allowedPrefixes` claims.
- Default presign TTL 900s.
- AK/SK only in gateway config.
- No binary proxy through the gateway.

## Out of scope (MVP)

- Local metadata DB, trash/versioning, media transforms, non-S3 adapters, full RBAC, multipart GC cron.
