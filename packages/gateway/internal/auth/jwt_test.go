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
		wantErr bool
	}{
		{"service role", &Claims{Role: "service_role"}, "uploads", "a/b", false},
		{"dev role", &Claims{Role: "dev"}, "uploads", "a/b", false},
		{"bucket mismatch", &Claims{Bucket: "other"}, "uploads", "a/b", true},
		{"prefix ok", &Claims{AllowedPrefixes: []string{"users/1/"}}, "uploads", "users/1/x.png", false},
		{"prefix deny", &Claims{AllowedPrefixes: []string{"users/1/"}}, "uploads", "users/2/x.png", true},
		{"no prefix restriction", &Claims{Bucket: "uploads"}, "uploads", "any/key", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AuthorizeBucketKey(tc.claims, tc.bucket, tc.key, true)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestParseJWT(t *testing.T) {
	secret := "test-secret"
	a := New(secret, false)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Bucket:          "uploads",
		AllowedPrefixes: []string{"avatar/"},
		Role:            "authenticated",
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := a.parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Bucket != "uploads" || claims.AllowedPrefixes[0] != "avatar/" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
