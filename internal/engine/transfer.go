package engine

import (
	"context"
	"io"
)

// TransferObject copies an object between buckets, using server-side copy when
// both sides share the same engine or streaming read+put across engines.
func TransferObject(ctx context.Context, srcEng, dstEng Engine, srcBucket, srcKey, dstBucket, dstKey string) error {
	if srcEng == dstEng {
		return srcEng.CopyObject(ctx, srcBucket, srcKey, dstBucket, dstKey)
	}
	rc, info, err := srcEng.GetObject(ctx, srcBucket, srcKey)
	if err != nil {
		return err
	}
	defer rc.Close()
	return dstEng.PutObject(ctx, dstBucket, dstKey, info.ContentType, "", rc, info.Metadata)
}

// TransferObjectFromReader puts bytes into dst without a source object (tests).
func TransferObjectFromReader(ctx context.Context, dstEng Engine, dstBucket, dstKey, contentType string, body io.Reader) error {
	return dstEng.PutObject(ctx, dstBucket, dstKey, contentType, "", body, nil)
}
