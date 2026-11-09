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
    ├ JWT + bucket / scope / key prefix authorization
    ├ Bucket upload policy (maxUploadBytes, allowedContentTypes)
    ├ Resolve minio-go client from bucket config
    ├ Issue S3 presigned URL (ContentType + Content-Length + metadata)
    ↓② Return uploadUrl / uploadId / part URLs (batched)
Frontend SDK
    ↓③ PUT bytes straight to object storage (+ x-amz-meta-*), parts in parallel
Object storage → CDN (Nginx → RustFS in development)
Business backend (packages/go-sdk): same presign control plane + direct PUT/GET;
persist bucket + key; call /object/head when metadata is needed
```

## ObjectAdapter

```go
type ObjectAdapter interface {
	PresignPut(ctx context.Context, bucket, key, contentType string, size int64, meta map[string]string, expiry time.Duration) (string, error)
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
	Ping(ctx context.Context, bucket string) error
}
```

Adapters wrap storage errors with `adapter.ErrNotFound`, `ErrInvalid`, `ErrAccessDenied` or `ErrPrecondition`; handlers map these to HTTP status codes without depending on a specific client library. Unclassified errors become a generic 500 (details are logged with the request id, never returned to clients).

## Security

- `JWT_SECRET` is required. Unauthenticated mode needs an explicit `ALLOW_NO_JWT=true` (local dev only).
- JWTs must be HS256 and carry `exp`; `iss` / `aud` are enforced when `JWT_ISSUER` / `JWT_AUDIENCE` are set.
- Claims: `bucket`, `allowedPrefixes` (directory boundaries: `users/1` grants `users/1/...`, not `users/10/...`), `scopes` (`read` / `write`), `role` (`service_role` bypasses checks — backends only, never browsers).
- Keys containing `.`/`..`/empty segments, a leading `/` or control characters are rejected so CDNs and browsers cannot normalize them into another user's path.
- Presign TTL defaults to 900s and is capped by `MAX_EXPIRE_SEC` (default 86400, S3 max 604800).
- Optional per-bucket `maxUploadBytes` (size signed into PUT URLs; multipart objects re-checked and deleted on complete) and `allowedContentTypes`.
- CORS origins come from `CORS_ALLOWED_ORIGINS`; JSON bodies are capped by `MAX_BODY_BYTES`.
- AK/SK only in gateway config. No binary proxy through the gateway.

### Public vs private buckets

`cdnUrl` is only meaningful for buckets that are publicly readable (anonymous read policy or a public CDN origin). In such a bucket **every** object is readable by anyone who knows the key, so prefix and scope checks only protect writes and presigned GETs. Keep private data in a separate bucket **without** an anonymous policy and without `cdnDomain`, and serve it via `/presign/get`. The Docker dev stack makes `uploads` public for convenience.

## Operations

- `GET /health`: liveness (process up).
- `GET /ready`: readiness — checks every configured bucket is reachable (503 otherwise).
- Every response carries `X-Request-Id`; access logs and error logs are JSON with `request_id`.
- Graceful shutdown on SIGINT/SIGTERM (20s drain); server read/write/idle timeouts are set.

## Out of scope (MVP)

- Local metadata DB, trash/versioning, media transforms, non-S3 adapters, full RBAC, multipart GC cron.
