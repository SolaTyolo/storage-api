package transform

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Crop mode (similar to Cloudinary c_).
type Crop string

const (
	CropScale Crop = "scale" // Scale to fit within w×h (default)
	CropFit   Crop = "fit"   // Same as scale
	CropFill  Crop = "fill"  // Cover w×h then center-crop
	CropPad   Crop = "pad"   // Fit inside w×h with padding
	CropThumb Crop = "thumb" // Smart thumbnail
)

// Params are passed via query string, e.g. ?w=200&h=200&c=fill&q=80&f=webp&t=1.5
type Params struct {
	Width   int
	Height  int
	Crop    Crop
	Quality int     // 1–100 for JPEG/WebP output
	Format  string  // auto | jpg | jpeg | png | webp
	TimeSec float64 // Video frame time in seconds (param t)
	Page    int     // PDF/Office page number (param page, default 1)
	DPI     int     // PDF render DPI (param dpi, default 150)
}

func ParseParams(q url.Values, maxEdge int) (Params, error) {
	p := Params{
		Crop:    CropScale,
		Quality: 85,
		Format:  "auto",
		TimeSec: 1,
	}

	if w := q.Get("w"); w != "" {
		n, err := strconv.Atoi(w)
		if err != nil || n < 0 {
			return p, fmt.Errorf("invalid w")
		}
		p.Width = clamp(n, maxEdge)
	}
	if h := q.Get("h"); h != "" {
		n, err := strconv.Atoi(h)
		if err != nil || n < 0 {
			return p, fmt.Errorf("invalid h")
		}
		p.Height = clamp(n, maxEdge)
	}

	switch strings.ToLower(strings.TrimSpace(q.Get("c"))) {
	case "", "scale":
		p.Crop = CropScale
	case "fit":
		p.Crop = CropFit
	case "fill":
		p.Crop = CropFill
	case "pad":
		p.Crop = CropPad
	case "thumb":
		p.Crop = CropThumb
	default:
		return p, fmt.Errorf("invalid c: use scale, fit, fill, pad, thumb")
	}

	if qv := q.Get("q"); qv != "" {
		n, err := strconv.Atoi(qv)
		if err != nil || n < 1 || n > 100 {
			return p, fmt.Errorf("invalid q: 1-100")
		}
		p.Quality = n
	}

	if f := strings.ToLower(strings.TrimSpace(q.Get("f"))); f != "" {
		switch f {
		case "auto", "jpg", "jpeg", "png", "webp":
			p.Format = f
		default:
			return p, fmt.Errorf("invalid f: auto, jpg, png, webp")
		}
	}

	if t := q.Get("t"); t != "" {
		sec, err := strconv.ParseFloat(t, 64)
		if err != nil || sec < 0 {
			return p, fmt.Errorf("invalid t")
		}
		p.TimeSec = sec
	}

	if pg := q.Get("page"); pg != "" {
		n, err := strconv.Atoi(pg)
		if err != nil || n < 1 {
			return p, fmt.Errorf("invalid page")
		}
		p.Page = n
	}
	if dpi := q.Get("dpi"); dpi != "" {
		n, err := strconv.Atoi(dpi)
		if err != nil || n < 36 || n > 600 {
			return p, fmt.Errorf("invalid dpi")
		}
		p.DPI = n
	}

	return p, nil
}

func clamp(n, max int) int {
	if max > 0 && n > max {
		return max
	}
	return n
}
