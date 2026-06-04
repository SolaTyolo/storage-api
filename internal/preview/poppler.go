package preview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// PopplerWorker 调用 Poppler 预览 sidecar（开源 Poppler + 薄 HTTP 封装）
type PopplerWorker struct {
	baseURL    string
	httpClient *http.Client
}

func NewPopplerWorker(baseURL string) *PopplerWorker {
	return &PopplerWorker{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// PDFToJPEG 将 PDF 指定页渲染为 JPEG
func (p *PopplerWorker) PDFToJPEG(ctx context.Context, pdf []byte, page, dpi int) ([]byte, error) {
	if page <= 0 {
		page = 1
	}
	if dpi <= 0 {
		dpi = 150
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "document.pdf")
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(pdf); err != nil {
		return nil, err
	}
	_ = w.WriteField("page", fmt.Sprintf("%d", page))
	_ = w.WriteField("dpi", fmt.Sprintf("%d", dpi))
	if err = w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/render", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("poppler worker: %s (%s)", res.Status, string(b))
	}
	return io.ReadAll(res.Body)
}
