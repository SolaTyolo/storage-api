package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var allowedJWTRoles = map[string]struct{}{
	"service_role":  {},
	"authenticated": {},
	"anon":          {},
}

func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

func (h *Handler) validJWT(token string) bool {
	if h.cfg.JWTSecret == "" || token == "" {
		return false
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	claims := jwt.MapClaims{}
	parsed, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		return false
	}
	if exp, err := claims.GetExpirationTime(); err != nil || exp == nil || exp.Time.Before(time.Now()) {
		return false
	}
	role, _ := claims["role"].(string)
	if role == "" {
		return true
	}
	_, ok := allowedJWTRoles[role]
	return ok
}

func (h *Handler) authorized(r *http.Request) bool {
	if authExempt(r) {
		return true
	}
	got := extractAPIKey(r)
	if len(h.cfg.APIKeys()) > 0 && matchAPIKey(got, h.cfg.APIKeys()) {
		return true
	}
	if h.cfg.JWTSecret != "" && looksLikeJWT(got) && h.validJWT(got) {
		return true
	}
	if len(h.cfg.APIKeys()) == 0 && h.cfg.JWTSecret == "" {
		return true
	}
	return false
}
