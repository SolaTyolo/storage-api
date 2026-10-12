package objsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError is returned when the gateway responds with a non-2xx status.
type APIError struct {
	Status  int
	Code    string
	Message string
	Body    []byte
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("gateway HTTP %d", e.Status)
}

type gatewayAPI struct {
	endpoint string
	token    string
	http     HTTPDoer
}

func newGatewayAPI(endpoint, token string, httpClient HTTPDoer) *gatewayAPI {
	return &gatewayAPI{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		http:     httpClient,
	}
}

func (a *gatewayAPI) post(ctx context.Context, path string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	res, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		ae := &APIError{Status: res.StatusCode, Body: data}
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &parsed) == nil {
			ae.Code = parsed.Error
			ae.Message = parsed.Message
		}
		return ae
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

type bucketKeyExpire struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	ContentType string `json:"contentType,omitempty"`
	Meta        Meta   `json:"meta,omitempty"`
	ExpireSec   int    `json:"expireSec,omitempty"`
}

func (a *gatewayAPI) presignPut(ctx context.Context, in bucketKeyExpire) (*PresignPutResponse, error) {
	var out PresignPutResponse
	if err := a.post(ctx, "/presign/put", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *gatewayAPI) presignGet(ctx context.Context, bucket, key string, expireSec int) (*PresignGetResponse, error) {
	var out PresignGetResponse
	if err := a.post(ctx, "/presign/get", map[string]any{
		"bucket":    bucket,
		"key":       key,
		"expireSec": expireSec,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *gatewayAPI) multipartCreate(ctx context.Context, in bucketKeyExpire) (*MultipartCreateResponse, error) {
	var out MultipartCreateResponse
	if err := a.post(ctx, "/multipart/create", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *gatewayAPI) multipartPresignPart(ctx context.Context, bucket, key, uploadID string, partNumber, expireSec int) (*MultipartPresignPartResponse, error) {
	var out MultipartPresignPartResponse
	if err := a.post(ctx, "/multipart/presign-part", map[string]any{
		"bucket":     bucket,
		"key":        key,
		"uploadId":   uploadID,
		"partNumber": partNumber,
		"expireSec":  expireSec,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *gatewayAPI) multipartComplete(ctx context.Context, bucket, key, uploadID string, parts []CompletePart) (*MultipartCompleteResponse, error) {
	var out MultipartCompleteResponse
	if err := a.post(ctx, "/multipart/complete", map[string]any{
		"bucket":   bucket,
		"key":      key,
		"uploadId": uploadID,
		"parts":    parts,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *gatewayAPI) multipartAbort(ctx context.Context, bucket, key, uploadID string) error {
	return a.post(ctx, "/multipart/abort", map[string]any{
		"bucket":   bucket,
		"key":      key,
		"uploadId": uploadID,
	}, nil)
}

func (a *gatewayAPI) objectHead(ctx context.Context, bucket, key string) (*ObjectHeadResponse, error) {
	var out ObjectHeadResponse
	if err := a.post(ctx, "/object/head", map[string]string{
		"bucket": bucket,
		"key":    key,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
