package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Bucket struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Owner            *uuid.UUID `json:"owner,omitempty"`
	Public           bool       `json:"public"`
	FileSizeLimit    *int64     `json:"file_size_limit,omitempty"`
	AllowedMimeTypes []string   `json:"allowed_mime_types,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ObjectMetadata struct {
	MimeType        string         `json:"mimetype,omitempty"`
	Size            int64          `json:"size,omitempty"`
	ETag            string         `json:"etag,omitempty"`
	S3Key  string         `json:"s3_key,omitempty"`
	Custom map[string]any `json:"-"`
}

func (m ObjectMetadata) ToMap() map[string]any {
	b, _ := json.Marshal(m)
	var base map[string]any
	_ = json.Unmarshal(b, &base)
	for k, v := range m.Custom {
		base[k] = v
	}
	if base == nil {
		base = map[string]any{}
	}
	return base
}

type StorageObject struct {
	ID             uuid.UUID      `json:"id"`
	BucketID       string         `json:"bucket_id"`
	Name           string         `json:"name"`
	Owner          *uuid.UUID     `json:"owner,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastAccessedAt time.Time      `json:"last_accessed_at"`
	Metadata       ObjectMetadata `json:"metadata"`
}
