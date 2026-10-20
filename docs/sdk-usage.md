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
git tag sdk-v0.1.0
git push origin sdk-v0.1.0
```

3. Repo secret **`NPM_TOKEN`** must be an npm access token that can publish under your npm account (same as packages like `formula-edit-lark`).

Manual run: Actions → **Publish SDK to npm** → **Run workflow**.

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

// Large file multipart
const big = await storage.multipartUpload(file, {
  key: "video/xxx.mp4",
  contentType: "video/mp4",
  partSize: 5 * 1024 * 1024,
  onProgress: (ev) => console.log(ev.percent),
});

console.log(res.cdnUrl, big.etag);

// Private download
const url = await storage.getDownloadUrl("private/report.pdf");

// Metadata
const head = await storage.head("avatar/xxx.png");
```

## JWT claims

HS256 JWT signed with `JWT_SECRET`:

| Claim | Meaning |
|-------|---------|
| `bucket` | Optional single-bucket restriction |
| `allowedPrefixes` | Optional key prefix allow-list |
| `role` | `service_role` bypasses prefix checks |

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
