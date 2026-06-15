package api

import (
	"errors"
	"net/http"

	"github.com/SolaTyolo/storage-api/internal/authz"
)

func (h *Handler) requireAuthz(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authExempt(r) || h.authzBypass(r) {
			next.ServeHTTP(w, r)
			return
		}
		action, bucketID, objectPath := authz.ParseRequest(r)
		subject := authz.SubjectFromRequest(r, h.cfg.JWTSecret, h.cfg.APIKeys())
		resource := authz.Resource{
			Method:     r.Method,
			Path:       r.URL.Path,
			Action:     action,
			BucketID:   bucketID,
			ObjectPath: objectPath,
		}
		if err := h.authz.Authorize(r.Context(), subject, resource); err != nil {
			if errors.Is(err, authz.ErrDenied) {
				h.logWarn(r, "authz denied", "bucket", bucketID, "path", objectPath, "action", action, "error", err.Error())
				writeStorageErr(w, http.StatusForbidden, "access_denied", err.Error())
				return
			}
			h.logError(r, "authz check failed", "error", err.Error())
			writeStorageErr(w, http.StatusInternalServerError, "internal", "authorization check failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) authzBypass(r *http.Request) bool {
	if h.cfg.AuthzBypassAPIKey {
		got := extractAPIKey(r)
		if len(h.cfg.APIKeys()) > 0 && matchAPIKey(got, h.cfg.APIKeys()) {
			return true
		}
	}
	if h.cfg.AuthzBypassServiceRole && h.cfg.JWTSecret != "" {
		got := extractAPIKey(r)
		if looksLikeJWT(got) && h.jwtRole(got) == "service_role" {
			return true
		}
	}
	return false
}

func (h *Handler) jwtRole(token string) string {
	subject := authz.SubjectFromRequest(&http.Request{Header: http.Header{
		"Authorization": []string{"Bearer " + token},
	}}, h.cfg.JWTSecret, nil)
	return subject.Role
}
