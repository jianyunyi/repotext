package control

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"net/http"
	"strings"
)

//go:embed web/*.html web/*.js web/*.css
var pageFS embed.FS

func (a API) Handler() http.Handler {
	token := a.CSRFToken
	if token == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		token = base64.RawURLEncoding.EncodeToString(b)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.Handle("POST /api/v1/previews", loopbackOnly(http.HandlerFunc(a.preview), token))
	mux.Handle("POST /api/v1/tasks", loopbackOnly(http.HandlerFunc(a.confirm), token))
	mux.HandleFunc("GET /api/v1/tasks/{id}", a.get)
	mux.HandleFunc("GET /api/v1/tasks/{id}/report", a.report)
	mux.Handle("GET /api/v1/tasks/{id}/log", loopbackOnly(http.HandlerFunc(a.log), token))
	for _, asset := range []struct{ path, contentType string }{
		{"report.js", "text/javascript; charset=utf-8"},
		{"report.css", "text/css; charset=utf-8"},
	} {
		mux.HandleFunc("GET /assets/"+asset.path, func(w http.ResponseWriter, r *http.Request) {
			if !isLoopbackRequest(r) {
				writeError(w, http.StatusForbidden, "local_only")
				return
			}
			w.Header().Set("Content-Type", asset.contentType)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			body, _ := pageFS.ReadFile("web/" + asset.path)
			_, _ = w.Write(body)
		})
	}
	mux.Handle("DELETE /api/v1/tasks/{id}", loopbackOnly(http.HandlerFunc(a.delete), token))
	mux.Handle("POST /api/v1/tasks/{id}/cancel", loopbackOnly(http.HandlerFunc(a.cancel), token))
	mux.Handle("GET /api/v1/sessions", loopbackOnly(http.HandlerFunc(a.listSessions), token))
	mux.Handle("DELETE /api/v1/sessions/{id}", loopbackOnly(http.HandlerFunc(a.revokeSession), token))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) {
			writeError(w, http.StatusForbidden, "local_only")
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		b, _ := pageFS.ReadFile("web/index.html")
		page := strings.ReplaceAll(string(b), "__CSRF_TOKEN__", token)
		if a.RunnerEnabled {
			page = strings.ReplaceAll(page, "__EXECUTION_ENABLED__", "true")
		} else {
			page = strings.ReplaceAll(page, "__EXECUTION_ENABLED__", "false")
		}
		_, _ = w.Write([]byte(page))
	})
	return mux
}
