# Cross-engine copy / move

## Does storage-js support this?

**Yes.** The official SDK `copy()` and `move()` methods accept an optional third argument, `DestinationOptions`:

```typescript
await supabase.storage
  .from('rustfs:uploads')
  .copy('a.png', 'a.png', { destinationBucket: 'minio:avatars' })
```

`from()` sets the **source** bucket (sent as `bucketId` in the request body). The destination bucket is passed via `destinationBucket`. Bucket ids are plain strings, so `engine:bucket` encoding works in the SDK without raw `fetch`.

JSON sent by storage-js:

```json
{
  "bucketId": "rustfs:uploads",
  "sourceKey": "a.png",
  "destinationKey": "a.png",
  "destinationBucket": "minio:avatars"
}
```

References: [Supabase copy/move guide](https://supabase.com/docs/guides/storage/management/copy-move-objects), `StorageFileApi.copy` / `move` in storage-js.

## Implemented in this project

**Yes — already implemented.** `POST /storage/v1/object/copy` and `POST /storage/v1/object/move` accept the same body shape as storage-js, including optional `destinationBucket`.

| Step | Code |
|------|------|
| Parse `bucketId` + `destinationBucket` | `internal/api/object.go` — resolves each via `registry.Resolve` (`engine:bucket` supported) |
| Transfer bytes | `engine.TransferObject` in `internal/engine/transfer.go` |
| Same engine | S3 server-side `CopyObject` |
| Different engines | Stream `GetObject` → `PutObject` through the gateway |
| Move | Copy then `DeleteObject` on the source |

Example with the SDK (cross-engine):

```typescript
const { data, error } = await supabase.storage
  .from('rustfs:uploads')
  .copy('reports/q1.pdf', 'archive/q1.pdf', {
    destinationBucket: 'minio:archive',
  })
```

## Comparison with hosted Supabase Storage

| Capability | Hosted Supabase | This project |
|------------|-----------------|--------------|
| Cross **logical bucket** (`destinationBucket`) | ✅ SDK + API | ✅ |
| Cross **physical engine** (`rustfs:` → `minio:`) | ❌ single backend | ✅ gateway streaming |

[Issue #1049](https://github.com/supabase/supabase-js/issues/1049) reported hosted Storage ignoring `destinationBucket` at one point; that was a **server-side** bug, not a missing SDK API.

## Limitations

- Large cross-engine objects use API bandwidth (streaming, not fully buffered in memory).
- Cross-engine **move** is not atomic (copy succeeds, then source delete — failure leaves a duplicate).
- Destination bucket must exist and be writable by the configured engine credentials.

## Alternatives

1. **Presigned pipeline** — presigned GET on source, client PUT to destination (no gateway bytes).
2. **Engine-side replication** — bucket replication between compatible S3 backends, outside this API.
3. **ETL worker** — batch copy with credentials for all engines.
