package model

import (
	"time"

	"github.com/google/uuid"
)

// Bucket matches Supabase Storage bucket JSON.
type Bucket struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Public           bool      `json:"public"`
	Owner            string    `json:"owner,omitempty"`
	FileSizeLimit    *int64    `json:"file_size_limit"`
	AllowedMimeTypes []string  `json:"allowed_mime_types"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// FileObject matches Supabase list/info response.
type FileObject struct {
	Name           string         `json:"name"`
	ID             string         `json:"id"`
	UpdatedAt      time.Time      `json:"updated_at"`
	CreatedAt      time.Time      `json:"created_at"`
	LastAccessedAt time.Time      `json:"last_accessed_at"`
	Metadata       FileMetadata   `json:"metadata"`
}

type FileMetadata struct {
	ETag     string `json:"eTag,omitempty"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mimetype,omitempty"`
	CacheControl string `json:"cacheControl,omitempty"`
}

// ObjectRef identifies an object in a physical engine.
type ObjectRef struct {
	Engine      string
	Bucket      string
	Path        string
	ContentType string
	Size        int64
}

func ObjectUUID(engine, bucket, path string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(engine+"/"+bucket+"/"+path))
}
