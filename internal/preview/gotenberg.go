package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Gotenberg document conversion client — https://github.com/gotenberg/gotenberg (MIT)
type Gotenberg struct {
	baseURL    string
	httpClient *http.Client
}

func NewGotenberg(baseURL string) *Gotenberg {
	return &Gotenberg{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// ToPDF converts Office or other supported documents to PDF bytes.
func (g *Gotenberg) ToPDF(ctx context.Context, filename string, body io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("files", filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, body); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/forms/libreoffice/convert", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	res, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("gotenberg: %s (%s)", res.Status, string(b))
	}

	return firstFileFromZip(res.Body)
}

func firstFileFromZip(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, fmt.Errorf("gotenberg: empty zip response")
}
