# storage-api (Go + RustFS)

Supabase Storage SDK–compatible HTTP API that talks directly to physical storage engines (S3-compatible). No PostgreSQL, no logical buckets.

[中文文档](./README.zh-CN.md)

- **Multi-engine**: YAML config + `STORAGE_DEFAULT_ENGINE` env var
- **Bucket routing**: `engine:bucket` (e.g. `rustfs:uploads`); omit engine to use the default
- **Preview**: on-demand rendering for images, video, PDF, and Office (imgproxy, Gotenberg, Poppler, ffmpeg)

## Quick start

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

- Playground: http://localhost:8080/playground/
- Transform example: `GET /storage/v1/render/image/public/uploads/photo.png?width=320&height=200&resize=cover`

```bash
cp .env.example .env
go run ./cmd/server
```

## Configuration

`config/storage.yaml`:

```yaml
default_engine: rustfs
engines:
  rustfs:
    type: s3
    endpoint: http://localhost:9000
    region: us-east-1
    access_key_id: rustfsadmin
    secret_access_key: rustfsadmin
    path_style: true
```

Environment variables:

| Variable | Description |
|----------|-------------|
| `STORAGE_CONFIG_PATH` | Path to YAML config |
| `STORAGE_DEFAULT_ENGINE` | Override default engine |
| `UPLOAD_SIGNING_SECRET` | HMAC secret for signed URL tokens |

## Supabase SDK usage

```typescript
const supabase = createClient(url, key, {
  global: { fetch: customFetch },
});
// bucket id supports engine:bucket format
await supabase.storage.from('rustfs:uploads').upload('a.png', file);
await supabase.storage.from('uploads').download('a.png');
```

## Project layout

```
cmd/server/
internal/api/          # Supabase Storage HTTP API
internal/engine/       # Multi-engine registry and S3 driver
internal/transform/    # On-demand image transforms
internal/preview/      # PDF/Office preview pipeline
config/storage.yaml    # Local / non-Docker storage config
deploy/                # Docker Compose, Dockerfiles, container config
```

See [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) and [docs/TRANSFORM_BACKENDS.md](./docs/TRANSFORM_BACKENDS.md).

Agent instructions: [AGENTS.md](./AGENTS.md)

## License

MIT
