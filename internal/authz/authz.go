package authz

import (
	"context"
	"errors"
	"net/http"
)

var ErrDenied = errors.New("access denied")

// Action is a normalized storage operation for policy checks.
type Action string

const (
	ActionAdmin  Action = "admin"
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionList   Action = "list"
)

// Subject is the authenticated caller.
type Subject struct {
	Type   string         `json:"type"` // api_key | jwt | anonymous
	Role   string         `json:"role,omitempty"`
	UserID string         `json:"user_id,omitempty"`
	Claims map[string]any `json:"claims,omitempty"`
	APIKey bool           `json:"api_key,omitempty"`
}

// Resource is the storage target of an operation.
type Resource struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Action     Action `json:"action"`
	BucketID   string `json:"bucket_id"`
	ObjectPath string `json:"object_path,omitempty"`
}

// Request is sent to external HTTP authorizers.
type Request struct {
	Subject  Subject  `json:"subject"`
	Resource Resource `json:"resource"`
}

// Response is expected from external HTTP authorizers.
type Response struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Authorizer decides whether a subject may perform an action on a resource.
type Authorizer interface {
	Authorize(ctx context.Context, subject Subject, resource Resource) error
}

// Noop allows all requests (default when no authorizer is configured).
type Noop struct{}

func (Noop) Authorize(context.Context, Subject, Resource) error { return nil }

// NewFromConfig builds an authorizer from application config.
func NewFromConfig(httpURL string, timeoutSec int) Authorizer {
	if httpURL == "" {
		return Noop{}
	}
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	return &HTTP{URL: httpURL, TimeoutSec: timeoutSec}
}

// ParseRequest derives action and bucket/object from an incoming HTTP request.
func ParseRequest(r *http.Request) (Action, string, string) {
	path := r.URL.Path
	method := r.Method

	switch {
	case method == http.MethodGet && path == "/storage/v1/bucket",
		method == http.MethodHead && path == "/storage/v1/bucket":
		return ActionList, "", ""
	case method == http.MethodPost && path == "/storage/v1/bucket":
		return ActionAdmin, "", ""
	case stringsHasPrefix(path, "/storage/v1/bucket/"):
		bucketID := chiBucketID(path, "/storage/v1/bucket/")
		switch method {
		case http.MethodGet, http.MethodHead:
			return ActionRead, bucketID, ""
		case http.MethodPut, http.MethodDelete:
			return ActionAdmin, bucketID, ""
		case http.MethodPost:
			return ActionAdmin, bucketID, ""
		}
	case stringsHasPrefix(path, "/storage/v1/object/list/"),
		stringsHasPrefix(path, "/storage/v1/object/list-v2/"):
		bucketID := chiBucketID(path, prefixBeforeBucket(path))
		return ActionList, bucketID, ""
	case path == "/storage/v1/object/copy" || path == "/storage/v1/object/move":
		return ActionWrite, "", ""
	case stringsHasPrefix(path, "/storage/v1/render/"):
		bucketID, objectPath := renderBucketPath(path)
		return ActionRead, bucketID, objectPath
	case stringsHasPrefix(path, "/storage/v1/object/public/"),
		stringsHasPrefix(path, "/storage/v1/object/authenticated/"),
		stringsHasPrefix(path, "/storage/v1/object/sign/"),
		stringsHasPrefix(path, "/storage/v1/object/info/"):
		bucketID, objectPath := objectBucketPath(path)
		switch method {
		case http.MethodGet, http.MethodHead:
			return ActionRead, bucketID, objectPath
		case http.MethodPost:
			return ActionRead, bucketID, objectPath
		case http.MethodDelete:
			return ActionDelete, bucketID, objectPath
		}
	case stringsHasPrefix(path, "/storage/v1/object/upload/sign/"):
		bucketID, objectPath := objectBucketPath(path)
		return ActionWrite, bucketID, objectPath
	case stringsHasPrefix(path, "/storage/v1/object/"):
		bucketID, objectPath := objectBucketPath(path)
		switch method {
		case http.MethodGet, http.MethodHead:
			return ActionRead, bucketID, objectPath
		case http.MethodPost, http.MethodPut:
			return ActionWrite, bucketID, objectPath
		case http.MethodDelete:
			return ActionDelete, bucketID, objectPath
		}
	}
	return Action(method), "", ""
}

func prefixBeforeBucket(path string) string {
	if stringsHasPrefix(path, "/storage/v1/object/list-v2/") {
		return "/storage/v1/object/list-v2/"
	}
	return "/storage/v1/object/list/"
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func chiBucketID(path, prefix string) string {
	rest := path[len(prefix):]
	if i := indexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func objectBucketPath(path string) (string, string) {
	for _, prefix := range []string{
		"/storage/v1/object/public/",
		"/storage/v1/object/authenticated/",
		"/storage/v1/object/sign/",
		"/storage/v1/object/info/",
		"/storage/v1/object/upload/sign/",
		"/storage/v1/object/",
	} {
		if stringsHasPrefix(path, prefix) {
			rest := path[len(prefix):]
			if i := indexByte(rest, '/'); i >= 0 {
				return rest[:i], rest[i+1:]
			}
			return rest, ""
		}
	}
	return "", ""
}

func renderBucketPath(path string) (string, string) {
	for _, prefix := range []string{
		"/storage/v1/render/image/public/",
		"/storage/v1/render/image/authenticated/",
	} {
		if stringsHasPrefix(path, prefix) {
			rest := path[len(prefix):]
			if i := indexByte(rest, '/'); i >= 0 {
				return rest[:i], rest[i+1:]
			}
			return rest, ""
		}
	}
	return "", ""
}
