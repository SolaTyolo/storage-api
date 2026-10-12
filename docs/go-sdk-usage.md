# Go SDK (`packages/go-sdk`)

Server-side client for obj-gateway. Same model as `@obj-gateway/sdk`: JWT against the gateway control plane, object bytes go directly to S3-compatible storage via presigned URLs.

Module: `github.com/SolaTyolo/storage-api/packages/go-sdk`

## Install

```bash
go get github.com/SolaTyolo/storage-api/packages/go-sdk@latest
```

From this monorepo (local replace):

```bash
go get github.com/SolaTyolo/storage-api/packages/go-sdk@v0.0.0
# or in go.mod:
# require github.com/SolaTyolo/storage-api/packages/go-sdk v0.0.0
# replace github.com/SolaTyolo/storage-api/packages/go-sdk => ../storage-api/packages/go-sdk
```

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
| `Upload` / `UploadBytes` | `/presign/put` then PUT to object storage |
| `MultipartUpload` | create → presign-part → PUT parts → complete |
| `DownloadURL` | `/presign/get` |
| `Download` | presign get + HTTP GET (caller closes body) |
| `Head` | `/object/head` |

Persist `bucket + key` in your business DB. Use `cdnUrl` for public assets; use `DownloadURL` / `Download` for private objects.

## JWT

Issue an HS256 JWT with `JWT_SECRET`. For internal services, set `role` to `service_role` to bypass key-prefix checks (still subject to gateway auth being enabled).
