# AGENTS.md

Instructions for AI coding agents working in **storage-api**.

## Project overview

Supabase Storage SDK–compatible HTTP API (Go 1.25). Proxies directly to physical S3-compatible engines (RustFS, MinIO, AWS S3). **No PostgreSQL**, no logical bucket layer.

- API base: `/storage/v1`
- Multi-engine bucket routing: `engine:bucket` (first `:` splits); plain `uploads` uses default engine
- Preview/transform on read only — originals stored in object storage, no derivative files

Human docs: [README.md](README.md) (EN), [README.zh-CN.md](README.zh-CN.md) (ZH).

## Setup and commands

```bash
# Local run
cp .env.example .env
go run ./cmd/server

# Build
go build ./cmd/server
go build ./cmd/preview-worker

# Docker (full stack: RustFS, API, imgproxy, Gotenberg, preview-worker)
docker compose -f deploy/docker-compose.yml up -d --build

# Smoke test (API must be up)
./scripts/test-upload.sh [image-file]
```

| Endpoint | URL |
|----------|-----|
| API / Playground | http://localhost:8080/playground/ |
| Health | http://localhost:8080/health |

Config: `config/storage.yaml` (local), `deploy/storage.docker.yaml` (Docker).

## Repository layout

```
cmd/server/           Main API
cmd/preview-worker/   Poppler PDF→JPEG sidecar (GPL isolated)
internal/api/         Chi router, Supabase handlers
internal/engine/      Engine interface, S3 driver, registry
internal/transform/   Image/video transforms
internal/preview/     Gotenberg + Poppler orchestration
config/               Storage engine YAML (local)
deploy/               Docker Compose + Dockerfiles
docs/                 Architecture and transform docs
```

## Architecture rules

1. **No database** — do not add Postgres, migrations, or object metadata tables.
2. **SDK-first** — routes and JSON must match [supabase/storage-js](https://github.com/supabase/storage-js).
3. **Engine registry** — handlers call `registry.Resolve(bucketRef)`; never hardcode one S3 client.
4. **On-demand transforms** — do not persist thumbnails or previews to storage.
5. **Poppler isolation** — PDF rasterization stays in `cmd/preview-worker`; do not link Poppler into the main API binary.

## Code conventions

- **Language**: English for code comments and docs; Chinese only in `README.zh-CN.md`.
- **Handlers**: Supabase errors via `writeStorageErr()` — `{"statusCode","error","message"}`.
- **Scope**: Minimal diffs; match existing chi patterns in `internal/api/`.
- **New routes**: Register in `internal/api/handler.go` under `/storage/v1`.
- **New engine**: Implement `engine.Engine` (`internal/engine/types.go`), register in `load.go` + YAML.

## Common tasks

| Task | Where |
|------|-------|
| Add Supabase endpoint | `internal/api/handler.go`, `bucket.go` / `object.go` / `render.go` |
| Change bucket routing | `internal/engine/resolve.go`, `registry.go` |
| Image/video transform | `internal/transform/` |
| PDF/Office preview | `internal/preview/`, `internal/api/render.go` |
| Docker / compose | `deploy/` |

## Verification before finishing

```bash
go build ./cmd/server && go build ./cmd/preview-worker
```

- New comments/docs in English (except `README.zh-CN.md`)
- No secrets in commits (`.env`, credentials)
- Do not commit binaries `/server`, `/preview-worker`

## Do not

- Reintroduce PostgreSQL or logical buckets
- Break storage-js route shapes or response fields
- Add TUS/S3-passthrough/Iceberg APIs unless explicitly requested
- Commit or push unless the user asks

## Further reading

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — routes, engine config, design principles
- [docs/TRANSFORM_BACKENDS.md](docs/TRANSFORM_BACKENDS.md) — preview pipeline
- [deploy/README.md](deploy/README.md) — container stack
- [.cursor/README.md](.cursor/README.md) — Cursor rules and nested AGENTS.md
