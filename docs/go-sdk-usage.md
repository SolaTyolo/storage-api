# Go SDK (`packages/go-sdk`)

Server-side client for obj-gateway. Same model as `obj-gateway-sdk`: JWT against the gateway control plane, object bytes go directly to S3-compatible storage via presigned URLs.

Module: `github.com/SolaTyolo/storage-api/packages/go-sdk`

## Install

```bash
go get github.com/SolaTyolo/storage-api/packages/go-sdk@v0.2.0
# or
go get github.com/SolaTyolo/storage-api/packages/go-sdk@latest
```

In the consumer `go.mod`:

```go
require github.com/SolaTyolo/storage-api/packages/go-sdk v0.2.0
```

From this monorepo (local replace):

```bash
# go.mod
# require github.com/SolaTyolo/storage-api/packages/go-sdk v0.0.0
# replace github.com/SolaTyolo/storage-api/packages/go-sdk => ../storage-api/packages/go-sdk
```

## Release (CI)

Nested Go modules need a path-prefixed tag. For this package that is:

```text
packages/go-sdk/vX.Y.Z
```

**Not** `sdk-v*` (that prefix is for the TypeScript npm package).

Preferred: Actions → **Release Go SDK** → **Run workflow**, set `version` to e.g. `0.2.0`. The workflow runs tests, pushes `packages/go-sdk/v0.2.0`, and creates a GitHub Release.

Manual equivalent:

```bash
git tag -a packages/go-sdk/v0.2.0 -m "Release Go SDK v0.2.0"
git push origin packages/go-sdk/v0.2.0
```

Workflow: [`.github/workflows/release-go-sdk.yml`](../.github/workflows/release-go-sdk.yml).

## Quick start

```go
package main

import (
	"context"
	"log"

	objsdk "github.com/SolaTyolo/storage-api/packages/go-sdk"
)

func main() {
	c, err := objsdk.New(objsdk.Config{
		Endpoint:      "http://localhost:8080/api/v1",
		Token:         "service-jwt", // prefer role=service_role
		DefaultBucket: "uploads",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	res, err := c.UploadBytes(ctx, []byte("hello"), objsdk.UploadOptions{
		Key:         "reports/a.pdf",
		ContentType: "application/pdf",
		Meta:        objsdk.Meta{"source": "billing"},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(res.Bucket, res.Key, res.CDNURL)

	url, err := c.DownloadURL(ctx, "private/report.pdf", nil)
	if err != nil {
		log.Fatal(err)
	}
	_ = url

	head, err := c.Head(ctx, "reports/a.pdf", nil)
	if err != nil {
		log.Fatal(err)
	}
	_ = head
}
```

## API surface

| Method | Behavior |
|--------|----------|
| `PresignPut` | `/presign/put` only (return URL to browser / caller) |
| `Upload` / `UploadBytes` / `PutText` | `/presign/put` then PUT to object storage |
| `MultipartUpload` | create → presign-parts (batched) → parallel PUT parts with retries → complete |
| `DownloadURL` / `Download` / `GetText` | `/presign/get` (+ GET bytes) |
| `Head` / `HeadExists` | `/object/head` (`HeadExists` is false only on 404) |
| `List` / `ListAllKeys` | `/object/list` |
| `Delete` | `/object/delete` |
| `Copy` | `/object/copy` |
| `SyncPrefix` | list both prefixes, copy changed objects (size + ETag), optional orphan delete / dry run |
| `Mkdir` | zero-byte upload with key ending in `/` |

Persist `bucket + key` in your business DB. Use `cdnUrl` for public assets; use `DownloadURL` / `Download` for private objects.

### Multipart

```go
f, _ := os.Open("video.mp4")
st, _ := f.Stat()
res, err := c.MultipartUpload(ctx, f, st.Size(), objsdk.MultipartUploadOptions{
	UploadOptions: objsdk.UploadOptions{Key: "video/a.mp4", ContentType: "video/mp4"},
	PartSize:      16 << 20, // raised automatically beyond 10,000 parts
	Concurrency:   4,
	MaxRetries:    3,
})
```

Bodies implementing `io.ReaderAt` (e.g. `*os.File`, `*bytes.Reader`) are read in parallel without buffering; other readers buffer up to `Concurrency + 1` parts in memory.

### Sync

```go
res, err := c.SyncPrefix(ctx, "releases/v1/", "releases/current/", &objsdk.SyncOptions{
	DeleteOrphans: true,
	DryRun:        true, // inspect res.Copied / res.Skipped / res.Deleted first
})
```

### Timeouts

The default HTTP client has no overall timeout so large transfers are not cut off. Gateway calls are bounded by `Config.APITimeout` (default 30s); object transfers are bounded by the context you pass in. If you supply `Config.HTTPClient`, avoid setting `http.Client.Timeout`.

## JWT

Issue an HS256 JWT with `JWT_SECRET` and an `exp` claim. For internal services, set `role` to `service_role` to bypass key-prefix and scope checks (still subject to gateway auth being enabled).
