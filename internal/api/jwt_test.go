package api

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/SolaTyolo/storage-api/internal/config"
)

func TestValidJWTServiceRole(t *testing.T) {
	secret := "test-jwt-secret"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "service_role",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{cfg: configWithJWT(secret)}
	if !h.validJWT(signed) {
		t.Fatal("expected valid service_role JWT")
	}
}

func TestValidJWTRejectsExpired(t *testing.T) {
	secret := "test-jwt-secret"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "service_role",
		"exp":  time.Now().Add(-time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{cfg: configWithJWT(secret)}
	if h.validJWT(signed) {
		t.Fatal("expected expired JWT to fail")
	}
}

func TestValidJWTRejectsWrongSecret(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": "anon",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("other-secret"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{cfg: configWithJWT("test-jwt-secret")}
	if h.validJWT(signed) {
		t.Fatal("expected wrong secret to fail")
	}
}

func TestLooksLikeJWT(t *testing.T) {
	if !looksLikeJWT("a.b.c") {
		t.Fatal("expected JWT shape")
	}
	if looksLikeJWT("plain-api-key") {
		t.Fatal("API key should not look like JWT")
	}
}

func configWithJWT(secret string) config.Config {
	return config.Config{JWTSecret: secret}
}
