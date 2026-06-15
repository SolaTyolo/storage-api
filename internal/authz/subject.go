package authz

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// SubjectFromRequest builds a Subject from headers and optional JWT secret.
func SubjectFromRequest(r *http.Request, jwtSecret string, apiKeys []string) Subject {
	token := extractBearerOrKey(r)
	if token == "" {
		return Subject{Type: "anonymous"}
	}
	for _, k := range apiKeys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(k)) == 1 {
			return Subject{Type: "api_key", APIKey: true, Role: "service_role"}
		}
	}
	if jwtSecret != "" && strings.Count(token, ".") == 2 {
		return subjectFromJWT(token, jwtSecret)
	}
	return Subject{Type: "api_key", APIKey: true}
}

func subjectFromJWT(token, secret string) Subject {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	claims := jwt.MapClaims{}
	parsed, err := parser.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return Subject{Type: "jwt", Role: "invalid"}
	}
	sub := Subject{Type: "jwt", Claims: map[string]any(claims)}
	if role, ok := claims["role"].(string); ok {
		sub.Role = role
	}
	if uid, ok := claims["sub"].(string); ok {
		sub.UserID = uid
	}
	return sub
}

func extractBearerOrKey(r *http.Request) string {
	if k := r.Header.Get("apikey"); k != "" {
		return k
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}
