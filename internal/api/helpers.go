package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type storageError struct {
	StatusCode string `json:"statusCode"`
	Error      string `json:"error"`
	Message    string `json:"message"`
}

func writeStorageErr(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(storageError{
		StatusCode: strconv.Itoa(status),
		Error:      code,
		Message:    message,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func upsertHeader(r *http.Request) bool {
	v := r.Header.Get("x-upsert")
	return v == "true" || v == "1"
}
