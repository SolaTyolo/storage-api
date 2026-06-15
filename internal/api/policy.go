package api

import (
	"errors"
	"io"
	"strings"

	"github.com/SolaTyolo/storage-api/internal/engine"
)

var errFileTooLarge = errors.New("file size exceeds bucket limit")

func validateUploadPolicy(resolved engine.ResolvedBucket, contentType string, contentLength int64) error {
	if resolved.FileSizeLimit != nil && *resolved.FileSizeLimit > 0 && contentLength > *resolved.FileSizeLimit {
		return errFileTooLarge
	}
	if len(resolved.AllowedMimeTypes) == 0 {
		return nil
	}
	if !mimeAllowed(contentType, resolved.AllowedMimeTypes) {
		return errors.New("mime type not allowed for this bucket")
	}
	return nil
}

func mimeAllowed(contentType string, allowed []string) bool {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if ct == "" {
		ct = "application/octet-stream"
	}
	for _, pattern := range allowed {
		p := strings.ToLower(strings.TrimSpace(pattern))
		switch {
		case p == "*/*":
			return true
		case p == ct:
			return true
		case strings.HasSuffix(p, "/*"):
			prefix := strings.TrimSuffix(p, "/*")
			if strings.HasPrefix(ct, prefix+"/") {
				return true
			}
		}
	}
	return false
}

type limitedReadCloser struct {
	rc        io.ReadCloser
	remaining int64
}

func limitUploadBody(rc io.ReadCloser, maxSize int64) io.ReadCloser {
	if maxSize <= 0 {
		return rc
	}
	return &limitedReadCloser{rc: rc, remaining: maxSize + 1}
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, errFileTooLarge
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.rc.Read(p)
	l.remaining -= int64(n)
	if l.remaining <= 0 {
		return n, errFileTooLarge
	}
	return n, err
}

func (l *limitedReadCloser) Close() error {
	return l.rc.Close()
}
