# Architecture

## Overview

`storage-api` implements a **Supabase Storage-compatible REST API** that proxies directly to physical object stores. There is **no PostgreSQL** and no logical/metadata bucket layer.

```
Client (supabase-js storage SDK)
        │
        ▼
  /storage/v1/*  (Go API)
        │
        ├─ engine registry (YAML + STORAGE_DEFAULT_ENGINE)
        │
        └─ physical engines (S3-compatible: RustFS, MinIO, AWS S3, …)
```

## Bucket routing

Supabase assumes one physical backend per project. This service supports multiple engines via bucket id encoding:

| Bucket id | Resolves to |
|-----------|-------------|
| `uploads` | default engine → physical bucket `uploads` |
| `rustfs:uploads` | engine `rustfs` → physical bucket `uploads` |
| `minio:avatars` | engine `minio` → physical bucket `avatars` |

Parsing uses the **first** `:` as the engine/bucket separator.

## Configuration

```yaml
# config/storage.yaml
default_engine: rustfs
engines:
  rustfs:
    type: s3
    endpoint: http://rustfs:9000
    region: us-east-1
    access_key_id: rustfsadmin
    secret_access_key: rustfsadmin
    path_style: true
```

- `STORAGE_CONFIG_PATH` — path to YAML file
- `STORAGE_DEFAULT_ENGINE` — optional override for `default_engine`

Bucket metadata (`public`, `file_size_limit`, `allowed_mime_types`) is stored as `.__storage/bucket.json` inside each physical bucket.

## API surface (Supabase Storage)

Base path: `/storage/v1`

### Buckets

| Method | Path | SDK method |
|--------|------|------------|
| GET | `/bucket` | `listBuckets()` |
| POST | `/bucket` | `createBucket()` |
| GET | `/bucket/{id}` | `getBucket()` |
| PUT | `/bucket/{id}` | `updateBucket()` |
| DELETE | `/bucket/{id}` | `deleteBucket()` |
| POST | `/bucket/{id}/empty` | `emptyBucket()` |

### Objects

| Method | Path | SDK method |
|--------|------|------------|
| POST | `/object/{bucket}/{path}` | `upload()` |
| PUT | `/object/{bucket}/{path}` | `update()` |
| GET | `/object/{bucket}/{path}` | `download()` |
| GET | `/object/public/{bucket}/{path}` | `getPublicUrl()` |
| POST | `/object/list/{bucket}` | `list()` |
| DELETE | `/object/{bucket}` | `remove()` (body: `{prefixes:[]}`) |
| POST | `/object/copy` | `copy()` |
| POST | `/object/move` | `move()` |
| POST | `/object/sign/{bucket}/{path}` | `createSignedUrl()` |
| POST | `/object/upload/sign/{bucket}/{path}` | `createSignedUploadUrl()` |

### Image transform / preview

| Method | Path | Notes |
|--------|------|-------|
| GET | `/render/image/public/{bucket}/{path}` | Supabase transform query params |
| GET | `/render/image/authenticated/{bucket}/{path}` | Same, for private buckets |

Supabase params: `width`, `height`, `resize`, `format`, `quality`.

Extended params (PDF/Office/video): `page`, `dpi`, `t`.

Pipeline:

- **image** → imgproxy (optional) or internal imaging
- **video** → ffmpeg frame + imaging
- **PDF** → Poppler worker → imaging
- **Office** → Gotenberg → PDF → Poppler → imaging

## Docker stack (dev)

Compose file: [`deploy/docker-compose.yml`](../deploy/docker-compose.yml)

| Service | Role |
|---------|------|
| rustfs | S3-compatible object store |
| api | This service |
| imgproxy | Image transforms (reads S3 directly) |
| gotenberg | Office → PDF |
| preview-worker | PDF → JPEG (Poppler sidecar) |
| init-bucket | Creates `uploads` bucket via API |

## Design principles

1. **No derivative objects** — transforms are computed on read; originals only in S3.
2. **SDK-first** — route shapes and response fields match `storage-js`.
3. **Engine pluggable** — new backends implement `engine.Engine` and are registered in YAML.
