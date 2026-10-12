# storage-api (obj-gateway)

Presigned-URL gateway for S3-compatible object storage (RustFS / MinIO / OSS / COS). The Go gateway never receives file bytes — clients upload directly with short-lived URLs. Go module path: `github.com/SolaTyolo/storage-api`.

[中文文档](./README.zh-CN.md)

## Features

- Presigned PutObject + S3 multipart (create / part / complete / abort)
- JWT auth with optional bucket / key-prefix isolation
- Metadata via `x-amz-meta-*` + `HeadObject` (no metadata DB)
- TypeScript SDK `@obj-gateway/sdk` (browser / Node)
- Go SDK `packages/go-sdk` (backend services; same presign model)
- Dev stack: **RustFS** + Nginx CDN + gateway + browser playground

## Quick start

```bash
# Full local stack (RustFS + gateway + CDN + playground)
make docker-up
# Playground http://localhost:5173
# API        http://localhost:8080
# CDN        http://localhost:8081
# RustFS     http://localhost:9000  (console :9001, rustfsadmin/rustfsadmin)
```

Health: `GET http://localhost:8080/health`

## Repository layout

```
packages/gateway/      Go gateway (minio-go adapter, JWT, /api/v1)
packages/sdk/          TypeScript frontend SDK
packages/go-sdk/       Go backend SDK (presign + direct object I/O)
packages/playground/   Browser upload playground
docker/                Compose (RustFS), Dockerfiles, Nginx CDN
docs/                  architecture, OpenAPI, SDK usage
```

## HTTP API

All under `/api/v1` with `Authorization: Bearer <jwt>`:

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/presign/put` | Presigned upload URL |
| POST | `/multipart/create` | Start multipart |
| POST | `/multipart/presign-part` | Presigned part URL |
| POST | `/multipart/complete` | Complete multipart |
| POST | `/multipart/abort` | Abort multipart |
| POST | `/presign/get` | Presigned download URL |
| POST | `/object/head` | Object metadata |

See [docs/api-spec.yaml](docs/api-spec.yaml).

## Configuration

| Variable | Description |
|----------|-------------|
| `LISTEN_ADDR` | Default `:8080` |
| `JWT_SECRET` | HS256 secret (empty = open access, **dev only**) |
| `DEFAULT_EXPIRE_SEC` | Presign TTL (default 900) |
| `BUCKET_CONFIG` | JSON array of bucket endpoints (AK/SK, CDN domain) |

## SDKs

**TypeScript** (browser / Node):

```bash
cd packages/sdk && npm install && npm run build
```

Usage: [docs/sdk-usage.md](docs/sdk-usage.md). Publish to npm (tag `sdk-vX.Y.Z` + secret `NPM_TOKEN`): see [docs/sdk-usage.md](docs/sdk-usage.md#publish-ci).

**Go** (backend services — same presign flow, no AK/SK in business code):

```bash
cd packages/go-sdk && go test ./...
```

Usage: [docs/go-sdk-usage.md](docs/go-sdk-usage.md). Module: `github.com/SolaTyolo/storage-api/packages/go-sdk`.

## Gateway image (Tencent TCR)

On push to `main`/`master` (gateway paths) or tag `gateway-v*`, GitHub Actions builds `docker/gateway/Dockerfile` and pushes to TCR.

Repo secrets: `TCR_USERNAME`, `TCR_PASSWORD`. Optional vars: `TCR_REGISTRY` (default `ccr.ccs.tencentyun.com`), `TCR_NAMESPACE` (default `solat`), `TCR_IMAGE_NAME` (default `obj-gateway`).

Docker base images for local compose are overridable via `.env` (`GOLANG_IMAGE`, `ALPINE_IMAGE`, `NODE_IMAGE`, `NGINX_IMAGE`, …); see `.env.example`.

## Verify

```bash
make test
make build
cd packages/sdk && npm run build
```

## Docs

- [docs/architecture.md](docs/architecture.md)
- [docs/api-spec.yaml](docs/api-spec.yaml)
- [docs/sdk-usage.md](docs/sdk-usage.md)
- [AGENTS.md](AGENTS.md)

## License

MIT
