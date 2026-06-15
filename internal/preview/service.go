package preview

import (
	"context"
	"errors"
	"io"

	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/mime"
	"github.com/SolaTyolo/storage-api/internal/model"
)

var ErrNotSupported = errors.New("preview not supported for this media type")

type Service struct {
	registry  *engine.Registry
	gotenberg *Gotenberg
	poppler   *PopplerWorker
}

func New(registry *engine.Registry, gotenbergURL, popplerURL string) *Service {
	var g *Gotenberg
	var p *PopplerWorker
	if gotenbergURL != "" {
		g = NewGotenberg(gotenbergURL)
	}
	if popplerURL != "" {
		p = NewPopplerWorker(popplerURL)
	}
	return &Service{registry: registry, gotenberg: g, poppler: p}
}

func (s *Service) Rasterize(ctx context.Context, obj model.ObjectRef, page, dpi int) ([]byte, error) {
	kind := mime.Classify(obj.ContentType)
	if !mime.PreviewSupported(obj.ContentType) {
		return nil, ErrNotSupported
	}

	switch kind {
	case mime.KindImage, mime.KindVideo:
		return nil, ErrNotSupported
	case mime.KindPDF:
		return s.pdfPage(ctx, obj, page, dpi)
	case mime.KindDocument:
		return s.officePage(ctx, obj, page, dpi)
	default:
		return nil, ErrNotSupported
	}
}

func (s *Service) pdfPage(ctx context.Context, obj model.ObjectRef, page, dpi int) ([]byte, error) {
	if s.poppler == nil {
		return nil, errors.New("poppler preview worker not configured")
	}
	eng, err := s.registry.Engine(obj.Engine)
	if err != nil {
		return nil, err
	}
	body, _, err := eng.GetObject(ctx, obj.Bucket, obj.Path)
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

func (s *Service) officePage(ctx context.Context, obj model.ObjectRef, page, dpi int) ([]byte, error) {
	if s.gotenberg == nil {
		return nil, errors.New("gotenberg not configured")
	}
	if s.poppler == nil {
		return nil, errors.New("poppler preview worker not configured")
	}
	eng, err := s.registry.Engine(obj.Engine)
	if err != nil {
		return nil, err
	}
	body, _, err := eng.GetObject(ctx, obj.Bucket, obj.Path)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	name := obj.Path
	pdf, err := s.gotenberg.ToPDF(ctx, name, body)
	if err != nil {
		return nil, err
	}
	return s.poppler.PDFToJPEG(ctx, pdf, page, dpi)
}
