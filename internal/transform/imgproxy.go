package transform

import (
	"encoding/base64"
	"fmt"
	"strings"

	appconfig "github.com/postship/storage/internal/config"
)

// ImgproxyURL 生成 imgproxy 处理 URL（开发用 /insecure/；生产需 URL 签名）
func ImgproxyURL(cfg appconfig.Config, s3Bucket, s3Key string, p Params) (string, error) {
	base := strings.TrimRight(cfg.ImgproxyBaseURL, "/")
	if !cfg.ImgproxyInsecure {
		return "", fmt.Errorf("signed imgproxy URLs not implemented; set IMGPROXY_INSECURE=true for dev")
	}

	opts := imgproxyProcessing(p)
	if p.Quality > 0 && p.Quality != 85 {
		opts = fmt.Sprintf("%s/q:%d", opts, p.Quality)
	}
	switch strings.ToLower(p.Format) {
	case "webp":
		opts += "/f:webp"
	case "png":
		opts += "/f:png"
	case "jpg", "jpeg":
		opts += "/f:jpg"
	}

	src := fmt.Sprintf("s3://%s/%s", s3Bucket, s3Key)
	enc := base64.RawURLEncoding.EncodeToString([]byte(src))
	return fmt.Sprintf("%s/insecure/%s/%s", base, opts, enc), nil
}

func imgproxyProcessing(p Params) string {
	w, h := p.Width, p.Height

	switch p.Crop {
	case CropFill:
		if w > 0 && h > 0 {
			return fmt.Sprintf("rs:fill:%d:%d", w, h)
		}
	case CropThumb:
		if w > 0 && h > 0 {
			return fmt.Sprintf("rs:force:%d:%d", w, h)
		}
	case CropPad, CropFit, CropScale:
		if w > 0 && h > 0 {
			return fmt.Sprintf("rs:fit:%d:%d", w, h)
		}
	}
	if w > 0 && h > 0 {
		return fmt.Sprintf("rs:fit:%d:%d", w, h)
	}
	if w > 0 {
		return fmt.Sprintf("w:%d", w)
	}
	if h > 0 {
		return fmt.Sprintf("h:%d", h)
	}
	return "w:0"
}
