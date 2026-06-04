package uploadtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid or expired complete token")

type Claims struct {
	BucketID    string `json:"b"`
	ObjectName  string `json:"n"`
	S3Key       string `json:"k"`
	ContentType string `json:"c"`
	Exp         int64  `json:"e"`
}

func Issue(secret string, c Claims) (string, error) {
	if secret == "" {
		return "", errors.New("upload signing secret not configured")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(p))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return p + "." + sig, nil
}

func Verify(secret, token string) (Claims, error) {
	if secret == "" {
		return Claims{}, errors.New("upload signing secret not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Claims{}, ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalid
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Claims{}, ErrInvalid
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrInvalid
	}
	if time.Now().Unix() > c.Exp {
		return Claims{}, ErrInvalid
	}
	if c.BucketID == "" || c.ObjectName == "" || c.S3Key == "" {
		return Claims{}, ErrInvalid
	}
	return c, nil
}

func NewClaims(bucketID, objectName, s3Key, contentType string, expires time.Time) Claims {
	return Claims{
		BucketID:    bucketID,
		ObjectName:  objectName,
		S3Key:       s3Key,
		ContentType: contentType,
		Exp:         expires.Unix(),
	}
}

func (c Claims) String() string {
	return fmt.Sprintf("%s/%s", c.BucketID, c.ObjectName)
}
