package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey int

const claimsKey ctxKey = 1

// Scope values accepted in the "scopes" claim.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// Claims are JWT claims used for bucket/key prefix authorization.
type Claims struct {
	jwt.RegisteredClaims
	// Bucket restricts access to a single bucket; empty means any configured bucket.
	Bucket string `json:"bucket,omitempty"`
	// AllowedPrefixes lists key prefixes the subject may access. Each prefix is a
	// directory boundary: "users/1" grants "users/1/..." but not "users/10/...".
	// Empty means any key (authenticated user with no prefix isolation).
	AllowedPrefixes []string `json:"allowedPrefixes,omitempty"`
	// Scopes limits operations to "read" and/or "write". Empty means both.
	Scopes []string `json:"scopes,omitempty"`
	// Role is optional (e.g. service_role bypasses prefix and scope checks).
	Role string `json:"role,omitempty"`
}

// Options configures an Authenticator.
type Options struct {
	Secret string
	// AllowNoJWT accepts unauthenticated requests when Secret is empty (local dev only).
	AllowNoJWT bool
	// Issuer and Audience are enforced when non-empty.
	Issuer   string
	Audience string
}

// Authenticator validates Bearer JWTs.
type Authenticator struct {
	secret     []byte
	allowNoJWT bool
	parser     *jwt.Parser
}

// New creates an authenticator.
func New(opts Options) *Authenticator {
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	}
	if opts.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(opts.Issuer))
	}
	if opts.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(opts.Audience))
	}
	return &Authenticator{
		secret:     []byte(opts.Secret),
		allowNoJWT: opts.AllowNoJWT,
		parser:     jwt.NewParser(parserOpts...),
	}
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

// AuthorizeBucketKey checks bucket, scope and key against claims.
// key may be an object key or a list prefix; it must already be validated
// (no leading "/", no "." or ".." segments).
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
	if len(claims.Scopes) > 0 {
		need := ScopeRead
		if write {
			need = ScopeWrite
		}
		if !slices.Contains(claims.Scopes, need) {
			return fmt.Errorf("%s scope required", need)
		}
	}
	if len(claims.AllowedPrefixes) == 0 {
		return nil
	}
	for _, p := range claims.AllowedPrefixes {
		p = strings.Trim(p, "/")
		if p == "" || strings.HasPrefix(key, p+"/") {
			return nil
		}
	}
	return fmt.Errorf("key prefix not allowed")
}

func (a *Authenticator) parse(tokenStr string) (*Claims, error) {
	tok, err := a.parser.ParseWithClaims(tokenStr, &Claims{}, func(*jwt.Token) (any, error) {
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
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": msg})
}
