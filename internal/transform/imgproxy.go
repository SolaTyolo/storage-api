package transform

import (
	"encoding/base64"
	"fmt"
)

func encodeBase64RawURL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
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
