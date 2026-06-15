# Deploy

Docker Compose stack for local development and testing.

## Quick start

From the repository root:

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

Or from this directory:

```bash
docker compose up -d --build
```

## Layout

| File | Description |
|------|-------------|
| `docker-compose.yml` | Full dev stack (RustFS, API, imgproxy, Gotenberg, preview-worker) |
| `Dockerfile` | Main API image |
| `preview-worker/Dockerfile` | Poppler rasterization sidecar |
| `storage.docker.yaml` | Storage engine config for containers (mounted into API) |

## Endpoints

- API: http://localhost:8080
- RustFS S3: http://localhost:9000
- imgproxy: http://localhost:8081
- Gotenberg: http://localhost:3000
- preview-worker: http://localhost:8090
