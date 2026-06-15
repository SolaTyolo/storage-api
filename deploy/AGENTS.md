---
description: Docker Compose stack and container images for storage-api
globs: deploy/**/*
alwaysApply: false
---

# Deploy conventions

- All Docker files live under `deploy/` (not repo root).
- Compose build context is repo root: `context: ..`, dockerfiles under `deploy/`.
- Container storage config: `deploy/storage.docker.yaml` mounted to `/etc/storage-api/storage.yaml`.

```bash
# From repo root
docker compose -f deploy/docker-compose.yml up -d --build

# From deploy/
docker compose up -d --build
```

When editing Dockerfiles, keep build stages minimal. API image needs `ffmpeg`; preview-worker needs `poppler-utils` only.

Update `deploy/README.md` if ports, services, or commands change.
