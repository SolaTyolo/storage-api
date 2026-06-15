package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3Config struct {
	Endpoint        string
	PublicEndpoint  string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	PathStyle       bool
}

type S3Engine struct {
	name           string
	client         *s3.Client
	publicEndpoint string
}

func NewS3Engine(name string, cfg S3Config) (*S3Engine, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Engine{name: name, client: client, publicEndpoint: cfg.PublicEndpoint}, nil
}

func (e *S3Engine) Name() string { return e.name }

func (e *S3Engine) CreateBucket(ctx context.Context, name string, meta BucketMeta) error {
	_, err := e.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		var exists *types.BucketAlreadyExists
		var owned *types.BucketAlreadyOwnedByYou
		if errors.As(err, &exists) || errors.As(err, &owned) {
			// bucket exists — still update metadata
		} else {
			return err
		}
	}
	return e.SetBucketMeta(ctx, name, meta, "")
}

func (e *S3Engine) DeleteBucket(ctx context.Context, name string) error {
	_, err := e.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	return err
}

func (e *S3Engine) ListBuckets(ctx context.Context) ([]string, error) {
	out, err := e.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, b := range out.Buckets {
		if b.Name != nil {
			names = append(names, *b.Name)
		}
	}
	return names, nil
}

func (e *S3Engine) HeadBucket(ctx context.Context, name string) error {
	_, err := e.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)})
	return err
}

func (e *S3Engine) GetBucketMeta(ctx context.Context, name string) (BucketMeta, string, error) {
	info, err := e.HeadObject(ctx, name, bucketMetaKey)
	if err != nil {
		if isNotFound(err) {
			return BucketMeta{}, "", nil
		}
		return BucketMeta{}, "", err
	}
	rc, _, err := e.GetObject(ctx, name, bucketMetaKey)
	if err != nil {
		return BucketMeta{}, "", err
	}
	defer rc.Close()
	var meta BucketMeta
	if err := json.NewDecoder(rc).Decode(&meta); err != nil {
		return BucketMeta{}, "", err
	}
	return meta, info.ETag, nil
}

func (e *S3Engine) SetBucketMeta(ctx context.Context, name string, meta BucketMeta, ifMatch string) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	in := &s3.PutObjectInput{
		Bucket:      aws.String(name),
		Key:         aws.String(bucketMetaKey),
		Body:        bytes.NewReader(b),
		ContentType: aws.String("application/json"),
	}
	if ifMatch != "" {
		in.IfMatch = aws.String(ifMatch)
	}
	_, err = e.client.PutObject(ctx, in)
	if err != nil && isPreconditionFailed(err) {
		return ErrPreconditionFailed
	}
	return err
}

func (e *S3Engine) EmptyBucket(ctx context.Context, name string) error {
	var token *string
	for {
		out, err := e.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(name),
			ContinuationToken: token,
		})
		if err != nil {
			return err
		}
		if len(out.Contents) == 0 {
			return nil
		}
		var keys []types.ObjectIdentifier
		for _, obj := range out.Contents {
			if obj.Key == nil || *obj.Key == bucketMetaKey {
				continue
			}
			keys = append(keys, types.ObjectIdentifier{Key: obj.Key})
		}
		if len(keys) == 0 {
			return nil
		}
		_, err = e.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(name),
			Delete: &types.Delete{Objects: keys},
		})
		if err != nil {
			return err
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		token = out.NextContinuationToken
	}
}

func (e *S3Engine) PutObject(ctx context.Context, bucket, key, contentType, cacheControl string, body io.Reader, metadata map[string]string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if cacheControl != "" {
		in.CacheControl = aws.String(cacheControl)
	}
	if len(metadata) > 0 {
		in.Metadata = metadata
	}
	_, err := e.client.PutObject(ctx, in)
	return err
}

func (e *S3Engine) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error) {
	out, err := e.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	return out.Body, objectInfoFromGet(out, key), nil
}

func (e *S3Engine) HeadObject(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	out, err := e.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	return objectInfoFromHead(out, key), nil
}

func (e *S3Engine) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := e.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (e *S3Engine) DeleteObjects(ctx context.Context, bucket string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	var objs []types.ObjectIdentifier
	for _, k := range keys {
		objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
	}
	_, err := e.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: objs},
	})
	return err
}

func (e *S3Engine) ListObjects(ctx context.Context, bucket, prefix string, limit, offset int) ([]ObjectInfo, error) {
	if limit <= 0 {
		limit = 100
	}
	var results []ObjectInfo
	var token *string
	skipped := 0

	for len(results) < limit {
		out, err := e.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range out.Contents {
			if obj.Key == nil || *obj.Key == bucketMetaKey || strings.HasPrefix(*obj.Key, ".__storage/") {
				continue
			}
			if skipped < offset {
				skipped++
				continue
			}
			info := ObjectInfo{Path: *obj.Key}
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.ETag != nil {
				info.ETag = strings.Trim(*obj.ETag, `"`)
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			results = append(results, info)
			if len(results) >= limit {
				break
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		token = out.NextContinuationToken
	}
	return results, nil
}

func (e *S3Engine) ListObjectsV2(ctx context.Context, bucket, prefix string, limit int, cursor string, withDelimiter bool) (ListPageV2, error) {
	if limit <= 0 {
		limit = 100
	}
	cont, err := decodeListCursor(cursor)
	if err != nil {
		return ListPageV2{}, err
	}
	var token *string
	if cont != "" {
		token = &cont
	}

	input := &s3.ListObjectsV2Input{
		Bucket:            aws.String(bucket),
		Prefix:            aws.String(prefix),
		MaxKeys:           aws.Int32(int32(limit)),
		ContinuationToken: token,
	}
	if withDelimiter {
		input.Delimiter = aws.String("/")
	}

	out, err := e.client.ListObjectsV2(ctx, input)
	if err != nil {
		return ListPageV2{}, err
	}

	page := ListPageV2{HasMore: aws.ToBool(out.IsTruncated)}
	if out.NextContinuationToken != nil {
		page.NextCursor = encodeListCursor(*out.NextContinuationToken)
	}

	for _, obj := range out.Contents {
		if obj.Key == nil || *obj.Key == bucketMetaKey || strings.HasPrefix(*obj.Key, ".__storage/") {
			continue
		}
		info := ObjectInfo{Path: *obj.Key}
		if obj.Size != nil {
			info.Size = *obj.Size
		}
		if obj.ETag != nil {
			info.ETag = strings.Trim(*obj.ETag, `"`)
		}
		if obj.LastModified != nil {
			info.LastModified = *obj.LastModified
		}
		page.Objects = append(page.Objects, info)
	}

	for _, cp := range out.CommonPrefixes {
		if cp.Prefix != nil {
			page.Folders = append(page.Folders, *cp.Prefix)
		}
	}

	return page, nil
}

func (e *S3Engine) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	src := fmt.Sprintf("%s/%s", srcBucket, srcKey)
	_, err := e.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(src),
	})
	return err
}

func (e *S3Engine) PresignGet(ctx context.Context, bucket, key string, expires time.Duration, downloadFilename string) (string, error) {
	presigner := s3.NewPresignClient(e.client)
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if downloadFilename != "" {
		input.ResponseContentDisposition = aws.String(`attachment; filename="` + downloadFilename + `"`)
	}
	out, err := presigner.PresignGetObject(ctx, input, s3.WithPresignExpires(expires))
	if err != nil {
		return "", err
	}
	return e.rewritePublicURL(out.URL), nil
}

func (e *S3Engine) PresignPut(ctx context.Context, bucket, key, contentType string, expires time.Duration) (string, error) {
	presigner := s3.NewPresignClient(e.client)
	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	out, err := presigner.PresignPutObject(ctx, input, s3.WithPresignExpires(expires))
	if err != nil {
		return "", err
	}
	return e.rewritePublicURL(out.URL), nil
}

func (e *S3Engine) rewritePublicURL(presigned string) string {
	if e.publicEndpoint == "" {
		return presigned
	}
	raw, err := url.Parse(presigned)
	if err != nil {
		return presigned
	}
	pub, err := url.Parse(e.publicEndpoint)
	if err != nil {
		return presigned
	}
	raw.Scheme = pub.Scheme
	raw.Host = pub.Host
	return raw.String()
}

func objectInfoFromGet(out *s3.GetObjectOutput, key string) ObjectInfo {
	info := ObjectInfo{Path: key, Metadata: out.Metadata}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, `"`)
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return info
}

func objectInfoFromHead(out *s3.HeadObjectOutput, key string) ObjectInfo {
	info := ObjectInfo{Path: key, Metadata: out.Metadata}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, `"`)
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return info
}

func isNotFound(err error) bool {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}
	return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "NoSuchKey")
}

func isPreconditionFailed(err error) bool {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "PreconditionFailed", "412":
			return true
		}
	}
	return strings.Contains(err.Error(), "PreconditionFailed")
}

var ErrNotFound = errors.New("not found")
