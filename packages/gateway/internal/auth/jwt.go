package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey int

const claimsKey ctxKey = 1

// Claims are JWT claims used for bucket/key prefix authorization.
type Claims struct {
	jwt.RegisteredClaims
	// Bucket restricts access to a single bucket; empty means any configured bucket.
	Bucket string `json:"bucket,omitempty"`
	// AllowedPrefixes lists key prefixes the subject may write/read.
	// Empty means any key (authenticated user with no prefix isolation).
	AllowedPrefixes []string `json:"allowedPrefixes,omitempty"`
	// Role is optional (e.g. service_role bypasses prefix checks).
	Role string `json:"role,omitempty"`
}

// Authenticator validates Bearer JWTs.
type Authenticator struct {
	secret     []byte
	allowNoJWT bool
}

// New creates an authenticator. If secret is empty and allowNoJWT, requests are allowed.
func New(secret string, allowNoJWT bool) *Authenticator {
	return &Authenticator{secret: []byte(secret), allowNoJWT: allowNoJWT}
}

// Middleware requires a valid JWT (or open access in allowNoJWT mode).
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.allowNoJWT && len(a.secret) == 0 {
			claims := &Claims{Role: "dev"}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		tokenStr, err := extractBearer(r)
		if err != nil {
			writeUnauthorized(w, err.Error())
			return
		}
		claims, err := a.parse(tokenStr)
		if err != nil {
			writeUnauthorized(w, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ClaimsFromContext returns JWT claims if present.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// AuthorizeBucketKey checks bucket and key against claims.
func AuthorizeBucketKey(claims *Claims, bucket, key string, write bool) error {
	if claims == nil {
		return fmt.Errorf("missing claims")
	}
	if claims.Role == "service_role" || claims.Role == "dev" {
		return nil
	}
	if claims.Bucket != "" && claims.Bucket != bucket {
		return fmt.Errorf("bucket not allowed")
	}
	if len(claims.AllowedPrefixes) == 0 {
		return nil
	}
	key = strings.TrimPrefix(key, "/")
	for _, p := range claims.AllowedPrefixes {
		p = strings.TrimPrefix(p, "/")
		if p == "" || strings.HasPrefix(key, p) {
			return nil
		}
	}
	_ = write
	return fmt.Errorf("key prefix not allowed")
}

func (a *Authenticator) parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected alg")
		}
		return a.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

func extractBearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing Authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", fmt.Errorf("expected Bearer token")
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if tok == "" {
		return "", fmt.Errorf("empty token")
	}
	return tok, nil
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"` + escapeJSON(msg) + `"}`))
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
