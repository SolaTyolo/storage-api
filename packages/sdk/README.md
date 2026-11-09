# obj-gateway-sdk

Frontend / Node SDK for [obj-gateway](https://github.com/SolaTyolo/storage-api): authenticate with JWT against the control plane, then upload/download object bytes **directly** to S3-compatible storage via presigned URLs. The gateway never proxies file bodies.

## Install

```bash
npm install obj-gateway-sdk
```

Requires Node.js ≥ 18 (or a modern browser bundler).

## Quick start

```ts
import { ObjGatewaySDK } from "obj-gateway-sdk";

const storage = new ObjGatewaySDK({
  gatewayEndpoint: "https://example.com/api/v1", // or /api/obj/api/v1 behind Traefik
  token: "<jwt>",
  defaultBucket: "uploads",
});

// Single-file upload (presigned PUT)
const res = await storage.upload(file, {
  key: "avatar/xxx.png",
  contentType: "image/png",
  meta: { originalName: "avatar.png" },
  onProgress: (ev) => console.log(ev.percent),
});
console.log(res.cdnUrl, res.key);

// Large file multipart (parallel parts with retries; abortable via `signal`)
const big = await storage.multipartUpload(file, {
  key: "video/xxx.mp4",
  contentType: "video/mp4",
  partSize: 8 * 1024 * 1024,
  concurrency: 4,
});

// Private download URL
const url = await storage.getDownloadUrl("private/report.pdf");

// Metadata
const head = await storage.head("avatar/xxx.png");

// List / delete / copy
const page = await storage.list({ prefix: "avatar/", delimiter: "/" });
await storage.copy({ srcKey: "avatar/xxx.png", destKey: "avatar/copy.png" });
await storage.delete("avatar/copy.png");
```

## JWT claims

HS256 JWT signed with the gateway `JWT_SECRET`; `exp` is required:

| Claim | Meaning |
|-------|---------|
| `bucket` | Optional single-bucket restriction |
| `allowedPrefixes` | Optional directory allow-list (`users/1` grants `users/1/...` only) |
| `scopes` | Optional `["read"]` / `["write"]` (empty = both) |
| `role` | `service_role` bypasses prefix and scope checks — never give it to browsers |

Business backends should mint a short-lived scoped JWT for browsers, or use `service_role` server-side (prefer the [Go SDK](https://github.com/SolaTyolo/storage-api/tree/main/packages/go-sdk)).

## Upload result

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

Persist `bucket + key` in your DB; use `cdnUrl` for public assets and `getDownloadUrl` for private ones.

## Docs

- Full usage & publish: [docs/sdk-usage.md](https://github.com/SolaTyolo/storage-api/blob/main/docs/sdk-usage.md)
- OpenAPI: [docs/api-spec.yaml](https://github.com/SolaTyolo/storage-api/blob/main/docs/api-spec.yaml)
- Architecture: [docs/architecture.md](https://github.com/SolaTyolo/storage-api/blob/main/docs/architecture.md)

## License

MIT
