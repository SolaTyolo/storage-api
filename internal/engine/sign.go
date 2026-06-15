package engine

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

type SignClaims struct {
	Engine string `json:"engine"`
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
	Op     string `json:"op"` // download | upload
	Exp    int64  `json:"exp"`
}

func IssueToken(secret string, claims SignClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(secret, enc)
	return enc + "." + sig, nil
}

func VerifyToken(secret, token string) (SignClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return SignClaims{}, errors.New("invalid token")
	}
	if !hmac.Equal([]byte(sign(secret, parts[0])), []byte(parts[1])) {
		return SignClaims{}, errors.New("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return SignClaims{}, err
	}
	var claims SignClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return SignClaims{}, err
	}
	if time.Now().Unix() > claims.Exp {
		return SignClaims{}, errors.New("token expired")
	}
	return claims, nil
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ObjectID(engineName, bucket, path string) string {
	return fmt.Sprintf("%s/%s/%s", engineName, bucket, path)
}
