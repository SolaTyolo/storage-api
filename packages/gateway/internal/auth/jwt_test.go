package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthorizeBucketKey(t *testing.T) {
	cases := []struct {
		name    string
		claims  *Claims
		bucket  string
		key     string
		write   bool
		wantErr bool
	}{
		{"service role", &Claims{Role: "service_role"}, "uploads", "a/b", true, false},
		{"dev role", &Claims{Role: "dev"}, "uploads", "a/b", true, false},
		{"bucket mismatch", &Claims{Bucket: "other"}, "uploads", "a/b", false, true},
		{"prefix ok", &Claims{AllowedPrefixes: []string{"users/1/"}}, "uploads", "users/1/x.png", true, false},
		{"prefix without slash ok", &Claims{AllowedPrefixes: []string{"users/1"}}, "uploads", "users/1/x.png", true, false},
		{"prefix leading slash ok", &Claims{AllowedPrefixes: []string{"/users/1/"}}, "uploads", "users/1/x.png", true, false},
		{"prefix deny", &Claims{AllowedPrefixes: []string{"users/1/"}}, "uploads", "users/2/x.png", true, true},
		{"prefix sibling deny", &Claims{AllowedPrefixes: []string{"users/1"}}, "uploads", "users/10/x.png", true, true},
		{"prefix itself deny", &Claims{AllowedPrefixes: []string{"users/1"}}, "uploads", "users/1", false, true},
		{"list root deny", &Claims{AllowedPrefixes: []string{"users/1"}}, "uploads", "", false, true},
		{"no prefix restriction", &Claims{Bucket: "uploads"}, "uploads", "any/key", true, false},
		{"read scope read ok", &Claims{Scopes: []string{ScopeRead}}, "uploads", "a", false, false},
		{"read scope write deny", &Claims{Scopes: []string{ScopeRead}}, "uploads", "a", true, true},
		{"write scope read deny", &Claims{Scopes: []string{ScopeWrite}}, "uploads", "a", false, true},
		{"both scopes", &Claims{Scopes: []string{ScopeRead, ScopeWrite}}, "uploads", "a", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AuthorizeBucketKey(tc.claims, tc.bucket, tc.key, tc.write)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func sign(t *testing.T, secret string, method jwt.SigningMethod, claims *Claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseJWT(t *testing.T) {
	secret := "test-secret"
	a := New(Options{Secret: secret})
	s := sign(t, secret, jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Bucket:          "uploads",
		AllowedPrefixes: []string{"avatar/"},
		Role:            "authenticated",
	})
	claims, err := a.parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Bucket != "uploads" || claims.AllowedPrefixes[0] != "avatar/" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseJWTRejects(t *testing.T) {
	secret := "test-secret"
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	cases := []struct {
		name  string
		opts  Options
		token func(t *testing.T) string
	}{
		{"missing exp", Options{Secret: secret}, func(t *testing.T) string {
			return sign(t, secret, jwt.SigningMethodHS256, &Claims{Role: "service_role"})
		}},
		{"expired", Options{Secret: secret}, func(t *testing.T) string {
			return sign(t, secret, jwt.SigningMethodHS256, &Claims{RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			}})
		}},
		{"wrong alg", Options{Secret: secret}, func(t *testing.T) string {
			return sign(t, secret, jwt.SigningMethodHS512, &Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}})
		}},
		{"wrong secret", Options{Secret: secret}, func(t *testing.T) string {
			return sign(t, "other", jwt.SigningMethodHS256, &Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}})
		}},
		{"wrong issuer", Options{Secret: secret, Issuer: "auth.example"}, func(t *testing.T) string {
			return sign(t, secret, jwt.SigningMethodHS256, &Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp, Issuer: "evil"}})
		}},
		{"wrong audience", Options{Secret: secret, Audience: "obj-gateway"}, func(t *testing.T) string {
			return sign(t, secret, jwt.SigningMethodHS256, &Claims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp, Audience: jwt.ClaimStrings{"other"}}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts).parse(tc.token(t)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
