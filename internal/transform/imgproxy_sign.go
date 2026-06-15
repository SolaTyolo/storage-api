package transform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func signImgproxyPath(key, salt, path string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(salt))
	_, _ = mac.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
