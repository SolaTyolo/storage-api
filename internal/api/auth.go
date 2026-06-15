package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (h *Handler) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		h.logWarn(r, "unauthorized API request", "reason", "invalid_credentials")
		writeStorageErr(w, http.StatusUnauthorized, "unauthorized", "Invalid API key or JWT")
	})
}

// authExempt skips API_KEY for Supabase-style public reads.
func authExempt(r *http.Request) bool {
	path := r.URL.Path
	method := r.Method

	switch {
	case strings.HasPrefix(path, "/storage/v1/object/public/"):
		return method == http.MethodGet || method == http.MethodHead
	case strings.HasPrefix(path, "/storage/v1/render/image/public/"):
		return method == http.MethodGet || method == http.MethodHead
	}
	return false
}

func extractAPIKey(r *http.Request) string {
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

func matchAPIKey(got string, keys []string) bool {
	for _, want := range keys {
		if validAPIKey(got, want) {
			return true
		}
	}
	return false
}

func validAPIKey(got, want string) bool {
	if want == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
