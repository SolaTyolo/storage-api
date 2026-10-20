# AGENTS.md

Instructions for AI coding agents working in **storage-api** (obj-gateway).

## Project overview

Presigned-URL gateway for S3-compatible storage. Go module: `github.com/SolaTyolo/storage-api`.

- Gateway never proxies file bytes — only issues presigned URLs and runs multipart control-plane calls.
- Metadata: `x-amz-meta-*` + `HeadObject` (no PostgreSQL / metadata DB).
- API base: `/api/v1`
- SDK: `packages/sdk` (`obj-gateway-sdk`); Go SDK: `packages/go-sdk`

## Setup and commands

```bash
# Docker stack (RustFS + gateway + Nginx CDN + playground)
make docker-up

# Gateway locally
cp .env.example .env
make run

# Build / test
make build
make test
cd packages/gateway && go build ./cmd/gateway
cd packages/go-sdk && go test ./...
cd packages/sdk && npm test && npm run build
```

| Endpoint | URL |
|----------|-----|
| Playground | http://localhost:5173 |
| API | http://localhost:8080 |
| Health | http://localhost:8080/health |
| CDN (dev) | http://localhost:8081 |
| RustFS S3 / Console | :9000 / :9001 |

## Repository layout

```
packages/gateway/cmd/gateway/     Main binary
packages/gateway/internal/adapter # minio-go ObjectAdapter
packages/gateway/internal/auth    # JWT + prefix checks
packages/gateway/internal/handler # /api/v1 handlers
packages/gateway/internal/config  # BUCKET_CONFIG loading
packages/sdk/src/                 # TypeScript SDK
packages/go-sdk/                  # Go backend SDK
packages/playground/              # Browser upload playground
docker/                           # Compose (RustFS) + Nginx + Dockerfiles
docs/                             # architecture, OpenAPI, SDK usage
```

## Architecture rules

1. **No database** — do not add Postgres or object metadata tables.
2. **No file streaming through gateway** — only presign / multipart control APIs.
3. **Adapter isolation** — handlers use `adapter.ObjectAdapter` via registry; do not hardcode one S3 client in handlers.
4. **minio-go** — domestic clouds via S3-compatible mode; force ContentType on PresignPut.
5. **English** in code comments and docs; Chinese only in `README.zh-CN.md`.
6. **Docker playground** uses RustFS; set `publicEndpoint` so browser presigned URLs use localhost, while `endpoint` stays the in-compose hostname.

## Code conventions

- Errors: JSON `{"error","message"}`.
- Auth: `Authorization: Bearer` JWT; claims `bucket`, `allowedPrefixes`, `role`.
- New routes: register under `/api/v1` in `packages/gateway/internal/handler`.
- Minimal diffs; match existing chi patterns.

## Verification before finishing

```bash
cd packages/gateway && go build ./cmd/gateway && go test ./...
cd packages/go-sdk && go test ./...
cd packages/sdk && npm test && npm run build
```

Do not commit secrets, `bin/`, or `packages/sdk/node_modules`.

## Do not

- Reintroduce Supabase `/storage/v1` compatibility or preview/transform pipelines unless explicitly requested
- Add a metadata database
- Proxy PUT/GET object bodies through the gateway
- Commit or push unless the user asks
