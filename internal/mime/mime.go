package mime

import "strings"

type MediaKind string

const (
	KindImage    MediaKind = "image"
	KindVideo    MediaKind = "video"
	KindAudio    MediaKind = "audio"
	KindPDF      MediaKind = "pdf"
	KindDocument MediaKind = "document" // Office 等，经 Gotenberg 转 PDF
	KindFile     MediaKind = "file"
)

func Classify(contentType string) MediaKind {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return KindImage
	case strings.HasPrefix(ct, "video/"):
		return KindVideo
	case strings.HasPrefix(ct, "audio/"):
		return KindAudio
	case ct == "application/pdf":
		return KindPDF
	case isOfficeMIME(ct):
		return KindDocument
	default:
		return KindFile
	}
}

func isOfficeMIME(ct string) bool {
	if strings.Contains(ct, "word") || strings.Contains(ct, "excel") ||
		strings.Contains(ct, "powerpoint") || strings.Contains(ct, "presentation") ||
		strings.Contains(ct, "spreadsheet") {
		return true
	}
	prefixes := []string{
		"application/vnd.openxmlformats-officedocument",
		"application/vnd.oasis.opendocument",
		"application/msword",
		"application/vnd.ms-excel",
		"application/vnd.ms-powerpoint",
		"application/rtf",
		"text/rtf",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}

func TransformSupported(contentType string) bool {
	k := Classify(contentType)
	return k == KindImage || k == KindVideo
}

// PreviewSupported 可通过 Gotenberg + Poppler 渲染首页预览
func PreviewSupported(contentType string) bool {
	k := Classify(contentType)
	return k == KindImage || k == KindVideo || k == KindPDF || k == KindDocument
}

// DeliverySupported 可走 /image 端点（含预览管线）
func DeliverySupported(contentType string) bool {
	return PreviewSupported(contentType)
}
