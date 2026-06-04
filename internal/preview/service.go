package preview

import (
	"context"
	"errors"
	"io"

	"github.com/postship/storage/internal/mime"
	"github.com/postship/storage/internal/s3client"
)

var ErrNotSupported = errors.New("preview not supported for this media type")

type Service struct {
	s3       *s3client.Client
	gotenberg *Gotenberg
	poppler  *PopplerWorker
}

func New(s3 *s3client.Client, gotenbergURL, popplerURL string) *Service {
	var g *Gotenberg
	var p *PopplerWorker
	if gotenbergURL != "" {
		g = NewGotenberg(gotenbergURL)
	}
	if popplerURL != "" {
		p = NewPopplerWorker(popplerURL)
	}
	return &Service{s3: s3, gotenberg: g, poppler: p}
}

// Rasterize 将对象转为 JPEG 栅格图（首页），供后续 w/h 变换
func (s *Service) Rasterize(ctx context.Context, s3Key, contentType, objectName string, page, dpi int) ([]byte, error) {
	kind := mime.Classify(contentType)
	if !mime.PreviewSupported(contentType) {
		return nil, ErrNotSupported
	}

	switch kind {
	case mime.KindImage, mime.KindVideo:
		return nil, ErrNotSupported // 由 transform 处理
	case mime.KindPDF:
		return s.pdfPage(ctx, s3Key, page, dpi)
	case mime.KindDocument:
		return s.officePage(ctx, s3Key, objectName, page, dpi)
	default:
		return nil, ErrNotSupported
	}
}

func (s *Service) pdfPage(ctx context.Context, s3Key string, page, dpi int) ([]byte, error) {
	if s.poppler == nil {
		return nil, errors.New("poppler preview worker not configured")
	}
	body, err := s.s3.Get(ctx, s3Key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	pdf, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return s.poppler.PDFToJPEG(ctx, pdf, page, dpi)
}

func (s *Service) officePage(ctx context.Context, s3Key, objectName string, page, dpi int) ([]byte, error) {
	if s.gotenberg == nil {
		return nil, errors.New("gotenberg not configured")
	}
	if s.poppler == nil {
		return nil, errors.New("poppler preview worker not configured")
	}
	body, err := s.s3.Get(ctx, s3Key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	pdf, err := s.gotenberg.ToPDF(ctx, objectName, body)
	if err != nil {
		return nil, err
	}
	return s.poppler.PDFToJPEG(ctx, pdf, page, dpi)
}
