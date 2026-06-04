package transform

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Crop 模式（类似 Cloudinary c_）
type Crop string

const (
	CropScale Crop = "scale" // 等比缩放到 w×h 框内（默认）
	CropFit   Crop = "fit"   // 同 scale
	CropFill  Crop = "fill"  // 铺满后居中裁剪
	CropPad   Crop = "pad"   // 等比放入框内并留白
	CropThumb Crop = "thumb" // 智能缩略图
)

// Params 通过 query 传入，例如 ?w=200&h=200&c=fill&q=80&f=webp&t=1.5
type Params struct {
	Width   int
	Height  int
	Crop    Crop
	Quality int     // 1–100，输出 JPEG/WebP 时使用
	Format  string  // auto | jpg | jpeg | png | webp
	TimeSec float64 // 视频截帧时间（秒），参数 t
	Page    int     // PDF/Office 页码，参数 page，默认 1
	DPI     int     // PDF 渲染 DPI，参数 dpi，默认 150
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
