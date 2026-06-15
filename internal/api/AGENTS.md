---
description: Supabase Storage HTTP handlers and routing
globs: internal/api/**/*.go
alwaysApply: false
---

# API handler conventions

- Base path: `/storage/v1` — match [storage-js](https://github.com/supabase/storage-js) paths and methods.
- When `API_KEY` is set, require `apikey` or `Authorization: Bearer` (constant-time compare).
- Resolve buckets with `h.registry.Resolve(r.Context(), bucketID)` before engine calls.
- Object paths: `strings.TrimPrefix(chi.URLParam(r, "*"), "/")`.
- Upload: honor `x-upsert: true`; support raw body and multipart (empty form field for file).
- Errors: `writeStorageErr(w, status, code, message)` — not ad-hoc JSON.

Register new routes in `handler.go`. Split handlers across `bucket.go`, `object.go`, `render.go`.

Render pipeline (`render.go`): image → imgproxy or transform; video → ffmpeg; PDF → Poppler; Office → Gotenberg → Poppler.
