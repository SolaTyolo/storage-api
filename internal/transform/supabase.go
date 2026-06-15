package transform

import (
	"fmt"
	"net/url"
	"strings"

	appconfig "github.com/SolaTyolo/storage-api/internal/config"
)

// ParseSupabaseTransform maps Supabase SDK query params to internal Params.
func ParseSupabaseTransform(q url.Values, maxEdge int) (Params, error) {
	mapped := url.Values{}
	if w := q.Get("width"); w != "" {
		mapped.Set("w", w)
	}
	if h := q.Get("height"); h != "" {
		mapped.Set("h", h)
	}
	if r := q.Get("resize"); r != "" {
		switch strings.ToLower(r) {
		case "cover":
			mapped.Set("c", "fill")
		case "contain":
			mapped.Set("c", "fit")
		case "fill":
			mapped.Set("c", "fill")
		default:
			mapped.Set("c", r)
		}
	}
	if f := q.Get("format"); f != "" {
		mapped.Set("f", f)
	}
	if quality := q.Get("quality"); quality != "" {
		mapped.Set("q", quality)
	}
	for _, k := range []string{"w", "h", "c", "q", "f", "t", "page", "dpi"} {
		if v := q.Get(k); v != "" {
			mapped.Set(k, v)
		}
	}
	return ParseParams(mapped, maxEdge)
}

// ImgproxyURL generates an imgproxy processing URL for a physical S3 bucket/key.
func ImgproxyURL(cfg appconfig.Config, physicalBucket, objectKey string, p Params) (string, error) {
	base := strings.TrimRight(cfg.ImgproxyBaseURL, "/")

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

	src := fmt.Sprintf("s3://%s/%s", physicalBucket, objectKey)
	encoded := encodeBase64RawURL(src)
	path := fmt.Sprintf("/%s/plain/%s", opts, encoded)

	if cfg.ImgproxyInsecure {
		return base + "/insecure" + path, nil
	}
	if cfg.ImgproxyKey == "" || cfg.ImgproxySalt == "" {
		return "", fmt.Errorf("IMGPROXY_KEY and IMGPROXY_SALT are required when IMGPROXY_INSECURE=false")
	}
	sig := signImgproxyPath(cfg.ImgproxyKey, cfg.ImgproxySalt, path)
	return base + "/" + sig + path, nil
}
