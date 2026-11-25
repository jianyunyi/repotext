package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Pairing struct {
	mu       sync.Mutex
	codeHash [32]byte
	expires  time.Time
	attempts int
	used     bool
	store    *SQLiteStore
}

func NewPairing(store *SQLiteStore, now time.Time) (*Pairing, string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return nil, "", err
	}
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")
	return &Pairing{codeHash: sha256.Sum256([]byte(code)), expires: now.Add(5 * time.Minute), store: store}, code, nil
}

func (p *Pairing) Consume(code string, now time.Time) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts++
	got := sha256.Sum256([]byte(code))
	if p.used || !now.Before(p.expires) || p.attempts > 5 || subtle.ConstantTimeCompare(got[:], p.codeHash[:]) != 1 {
		return "", errors.New("pairing rejected")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base32.StdEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	if err := p.store.CreateSession(context.Background(), hash[:], now.Add(30*24*time.Hour)); err != nil {
		return "", err
	}
	p.used = true
	return token, nil
}

func (p *Pairing) Valid(token string, now time.Time) bool {
	hash := sha256.Sum256([]byte(token))
	valid, err := p.store.SessionValid(context.Background(), hash[:], now)
	return err == nil && valid
}

func (p *Pairing) Revoke(token string) error {
	hash := sha256.Sum256([]byte(token))
	return p.store.RevokeSession(context.Background(), hash[:])
}

func NewLANHandler(api API, p *Pairing, allowedHosts []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		page, _ := pageFS.ReadFile("web/lan.html")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /pair", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Code string `json:"code"`
		}
		if p == nil || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		token, err := p.Consume(request.Code, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, "pairing_rejected")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "repotest_read", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int((30 * 24 * time.Hour).Seconds())})
		writeJSON(w, http.StatusOK, map[string]string{"status": "paired"})
	})
	mux.HandleFunc("GET /api/v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("repotest_read")
		if err != nil || p == nil || !p.Valid(cookie.Value, time.Now()) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		api.get(w, r)
	})
	mux.HandleFunc("GET /api/v1/tasks/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		if !lanSessionValid(w, r, p) {
			return
		}
		if api.Store == nil {
			writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
			return
		}
		store, ok := api.Store.(*SQLiteStore)
		if !ok {
			writeError(w, http.StatusNotFound, "report_not_found")
			return
		}
		body, err := store.GetReport(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "report_not_found")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("GET /api/v1/tasks/{id}/log", func(w http.ResponseWriter, r *http.Request) {
		if !lanSessionValid(w, r, p) {
			return
		}
		api.log(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	})
	return lanHostOnly(mux, allowedHosts)
}

func lanSessionValid(w http.ResponseWriter, r *http.Request, p *Pairing) bool {
	cookie, err := r.Cookie("repotest_read")
	if err != nil || p == nil || !p.Valid(cookie.Value, time.Now()) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}
