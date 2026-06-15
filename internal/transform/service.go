package transform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/SolaTyolo/storage-api/internal/config"
	"github.com/SolaTyolo/storage-api/internal/engine"
	"github.com/SolaTyolo/storage-api/internal/mime"
	"github.com/SolaTyolo/storage-api/internal/model"
)

var ErrNotSupported = errors.New("transform not supported for this media type")

type Service struct {
	registry *engine.Registry
	maxEdge  int
	ffmpeg   string
}

func New(cfg config.Config, registry *engine.Registry) *Service {
	return &Service{registry: registry, maxEdge: cfg.TransformMaxEdge, ffmpeg: cfg.FFmpegPath}
}

func (s *Service) RenderJPEG(jpeg []byte, p Params) ([]byte, string, error) {
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return nil, "", err
	}
	if p.Width > 0 || p.Height > 0 {
		img = applyCrop(img, p)
	}
	return encode(img, p)
}

func (s *Service) Render(ctx context.Context, obj model.ObjectRef, p Params) ([]byte, string, error) {
	kind := mime.Classify(obj.ContentType)
	if kind != mime.KindImage && kind != mime.KindVideo {
		return nil, "", ErrNotSupported
	}

	eng, err := s.registry.Engine(obj.Engine)
	if err != nil {
		return nil, "", err
	}

	var img image.Image

	switch kind {
	case mime.KindImage:
		img, err = s.loadImage(ctx, eng, obj.Bucket, obj.Path)
	case mime.KindVideo:
		img, err = s.loadVideoFrame(ctx, eng, obj.Bucket, obj.Path, p.TimeSec)
	}
	if err != nil {
		return nil, "", err
	}

	if p.Width > 0 || p.Height > 0 {
		img = applyCrop(img, p)
	}

	return encode(img, p)
}

func (s *Service) loadImage(ctx context.Context, eng engine.Engine, bucket, key string) (image.Image, error) {
	body, _, err := eng.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return imaging.Decode(body)
}

func (s *Service) loadVideoFrame(ctx context.Context, eng engine.Engine, bucket, key string, timeSec float64) (image.Image, error) {
	tmpDir, err := os.MkdirTemp("", "transform-video-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source")
	outPath := filepath.Join(tmpDir, "frame.jpg")

	body, _, err := eng.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(srcPath)
	if err != nil {
		body.Close()
		return nil, err
	}
	if _, err = io.Copy(f, body); err != nil {
		body.Close()
		f.Close()
		return nil, err
	}
	body.Close()
	f.Close()

	ss := fmt.Sprintf("%.3f", timeSec)
	cmd := exec.CommandContext(ctx, s.ffmpeg,
		"-y", "-ss", ss, "-i", srcPath,
		"-frames:v", "1", "-q:v", "2", outPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w (%s)", err, string(out))
	}
	return imaging.Open(outPath)
}

func applyCrop(img image.Image, p Params) image.Image {
	w, h := p.Width, p.Height
	if w <= 0 && h <= 0 {
		return img
	}
	if w <= 0 {
		return imaging.Resize(img, 0, h, imaging.Lanczos)
	}
	if h <= 0 {
		return imaging.Resize(img, w, 0, imaging.Lanczos)
	}

	switch p.Crop {
	case CropFill:
		return imaging.Fill(img, w, h, imaging.Center, imaging.Lanczos)
	case CropThumb:
		return imaging.Thumbnail(img, w, h, imaging.Lanczos)
	case CropPad:
		fitted := imaging.Fit(img, w, h, imaging.Lanczos)
		canvas := imaging.New(w, h, color.NRGBA{0, 0, 0, 0})
		b := fitted.Bounds()
		pt := image.Pt((w-b.Dx())/2, (h-b.Dy())/2)
		draw.Draw(canvas, b.Add(pt), fitted, b.Min, draw.Over)
		return canvas
	case CropFit, CropScale:
		return imaging.Fit(img, w, h, imaging.Lanczos)
	default:
		return imaging.Fit(img, w, h, imaging.Lanczos)
	}
}

func encode(img image.Image, p Params) ([]byte, string, error) {
	format := p.Format
	if format == "auto" {
		format = "jpeg"
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/png", nil
	case "webp":
		return nil, "", ErrNotSupported
	default:
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: p.Quality}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/jpeg", nil
	}
}
