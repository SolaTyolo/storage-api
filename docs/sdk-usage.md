# obj-gateway-sdk usage

## Install

```bash
npm install obj-gateway-sdk
# or from monorepo
npm install ../packages/sdk
```

## Publish (CI)

Publishing is handled by [`.github/workflows/publish-sdk.yml`](../.github/workflows/publish-sdk.yml).

1. Bump `packages/sdk/package.json` `version`.
2. Create and push a tag matching that version:

```bash
git tag sdk-v0.2.1
git push origin sdk-v0.2.1
```

3. Repo secret **`NPM_TOKEN`** must be an npm access token that can publish under your npm account (same as packages like `formula-edit-lark`).

Manual run: Actions → **Publish SDK to npm** → **Run workflow** with the `sdk-vX.Y.Z` tag to publish; the tag must match `package.json`.

## Playground (Docker)

```bash
make docker-up
# open http://localhost:5173 — uploads go gateway → RustFS
```

See [packages/playground/README.md](../packages/playground/README.md).

## Quick start

```ts
import { ObjGatewaySDK } from "obj-gateway-sdk";

const storage = new ObjGatewaySDK({
  gatewayEndpoint: "http://localhost:8080/api/v1",
  token: "user-jwt-token",
  defaultBucket: "uploads",
});

// Single-file upload (presigned PUT)
const res = await storage.upload(file, {
  key: "avatar/xxx.png",
  contentType: "image/png",
  meta: { originalName: "avatar.png" },
  onProgress: (ev) => console.log(ev.percent),
});

// Large file multipart: parts upload in parallel with per-part retries.
// partSize is raised automatically to stay within S3's 10,000-part limit.
const controller = new AbortController();
const big = await storage.multipartUpload(file, {
  key: "video/xxx.mp4",
  contentType: "video/mp4",
  partSize: 8 * 1024 * 1024,
  concurrency: 4,
  maxRetries: 3,
  signal: controller.signal, // controller.abort() also aborts the multipart upload server-side
  onProgress: (ev) => console.log(ev.percent),
});

console.log(res.cdnUrl, big.etag);

// Private download
const url = await storage.getDownloadUrl("private/report.pdf");

// Metadata
const head = await storage.head("avatar/xxx.png");

// List / delete / copy
const page = await storage.list({ prefix: "avatar/", delimiter: "/" });
const all = await storage.listAll("avatar/");
await storage.copy({ srcKey: "avatar/xxx.png", destKey: "avatar/backup.png" });
await storage.delete("avatar/backup.png");
```

Uploads send the file size to the gateway, which signs it as `Content-Length`; buckets with `maxUploadBytes` reject larger files before any bytes are sent. Storage CORS must expose the `ETag` header for multipart uploads.

Errors from the gateway are `GatewayApiError` (`status`, `code`, `body`); failed storage PUTs are `StorageHttpError` (`status`).

## JWT claims

HS256 JWT signed with `JWT_SECRET`; `exp` is required:

| Claim | Meaning |
|-------|---------|
| `bucket` | Optional single-bucket restriction |
| `allowedPrefixes` | Optional directory allow-list (`users/1` grants `users/1/...`, not `users/10/...`) |
| `scopes` | Optional `["read"]`, `["write"]` or both (empty = both) |
| `role` | `service_role` bypasses prefix and scope checks — backends only |

Browsers should get short-lived tokens scoped to the user's prefix, never `service_role`.

## Result shape

```ts
{
  bucket: string;
  key: string;
  cdnUrl: string;
  etag: string;
  size: number;
  contentType: string;
  meta: Record<string, string>;
}
```

Business backends should persist `bucket + key` only; use `cdnUrl` for public assets and `/presign/get` for private downloads.

For Go services, prefer [docs/go-sdk-usage.md](./go-sdk-usage.md) instead of calling the HTTP API by hand.
