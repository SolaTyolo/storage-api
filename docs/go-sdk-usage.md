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
| `MultipartUpload` | create → presign-part → PUT parts → complete |
| `DownloadURL` / `Download` / `GetText` | `/presign/get` (+ GET bytes) |
| `Head` / `HeadExists` | `/object/head` |
| `List` / `ListAllKeys` | `/object/list` |
| `Delete` | `/object/delete` |
| `Copy` / `SyncPrefix` | `/object/copy` (+ list/delete for sync) |
| `Mkdir` | zero-byte upload with key ending in `/` |

Persist `bucket + key` in your business DB. Use `cdnUrl` for public assets; use `DownloadURL` / `Download` for private objects.

## JWT

Issue an HS256 JWT with `JWT_SECRET`. For internal services, set `role` to `service_role` to bypass key-prefix checks (still subject to gateway auth being enabled).
