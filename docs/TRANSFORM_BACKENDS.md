# Open-source transform and preview stack

The API stores **original files only** in S3-compatible object storage. Thumbnails and previews are generated on demand via:

`GET /storage/v1/render/image/public|authenticated/{bucket}/{path}?width=&height=&resize=...`

## Components (all open source)

| Service | License | Role | Port (compose) |
|---------|---------|------|----------------|
| [imgproxy](https://github.com/imgproxy/imgproxy) | MIT | Image `width/height/resize/quality/format`; reads S3 directly | 8081 |
| [Gotenberg](https://github.com/gotenberg/gotenberg) | MIT | Office → PDF | 3000 |
| **preview-worker** | MIT (wrapper) | Poppler `pdftoppm`: PDF page → JPEG | 8090 |
| **Built-in transform** | — | Video ffmpeg frame extract; resize after PDF/Office rasterization | API process |

Poppler itself is **GPL-2.0** (`poppler-utils`). It runs only in the isolated preview-worker sidecar, not in the main API process.

## Routing by MIME type

```text
GET /render/image/.../bucket/path?width=200&height=200&resize=cover&page=1&dpi=150
        │
        ├─ image/*          ──► imgproxy (TRANSFORM_BACKEND=imgproxy) or built-in imaging
        ├─ video/*          ──► built-in ffmpeg + imaging
        ├─ application/pdf  ──► Poppler worker → imaging
        └─ Office/*         ──► Gotenberg → PDF → Poppler → imaging
```

Other types (zip, plain text, etc.): `415 Unsupported Media Type` — use direct object download.

## Environment variables

```env
TRANSFORM_BACKEND=imgproxy   # images via imgproxy; video/PDF still use API pipeline
IMGPROXY_BASE_URL=http://imgproxy:8080
IMGPROXY_INSECURE=true       # dev only; sign imgproxy URLs in production

GOTENBERG_URL=http://gotenberg:3000
POPPLER_WORKER_URL=http://preview-worker:8090
```

Supabase SDK query params (`width`, `height`, `resize`, `format`, `quality`) are supported. Extended params for PDF/video: `page`, `dpi`, `t`.

## Quick start

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

- API: http://localhost:8080
- imgproxy: http://localhost:8081
- Gotenberg: http://localhost:3000/health

## Production notes

1. **imgproxy**: set matching `IMGPROXY_KEY` / `IMGPROXY_SALT` on imgproxy and the API; set `IMGPROXY_INSECURE=false` — the API issues HMAC-signed URLs automatically.
2. **Gotenberg / preview-worker**: internal network only; do not expose to the public internet.
3. **API**: enforce bucket policy / JWT before serving `/render/image/...`.

## Unsupported formats

- Audio waveforms, HEIC (some source images can be handled by imgproxy)
- Complex CAD, encrypted PDF

Additional open-source services can be wired in via new API routes without schema changes.
