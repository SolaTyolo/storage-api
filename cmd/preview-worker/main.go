// Poppler preview sidecar — requires system poppler-utils (pdftoppm).
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	addr := getenv("HTTP_ADDR", ":8090")
	token := os.Getenv("SIDECAR_API_TOKEN")
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/v1/render", requireBearer(token, http.HandlerFunc(handleRender)))
	log.Printf("preview-worker (poppler) listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func requireBearer(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := bearerToken(r)
		if got == "" || got != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	page, _ := strconv.Atoi(r.FormValue("page"))
	if page <= 0 {
		page = 1
	}
	dpi, _ := strconv.Atoi(r.FormValue("dpi"))
	if dpi <= 0 {
		dpi = 150
	}

	tmp, err := os.MkdirTemp("", "pdf-in-*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmp)

	inPath := filepath.Join(tmp, "in.pdf")
	out, err := os.Create(inPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err = io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out.Close()

	outPrefix := filepath.Join(tmp, "page")
	cmd := exec.Command("pdftoppm",
		"-jpeg", "-singlefile",
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		"-r", strconv.Itoa(dpi),
		inPath, outPrefix,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		http.Error(w, fmt.Sprintf("pdftoppm: %v (%s)", err, string(out)), http.StatusInternalServerError)
		return
	}

	jpegPath := outPrefix + ".jpg"
	data, err := os.ReadFile(jpegPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
