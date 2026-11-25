package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

func TestReportAssetsAreEmbeddedAndLocalOnly(t *testing.T) {
	h := API{CSRFToken: testCSRFToken}.Handler()
	for _, asset := range []struct{ path, contentType, marker string }{
		{"/assets/report.js", "text/javascript; charset=utf-8", "RepoTestReport"},
		{"/assets/report.css", "text/css; charset=utf-8", ".report"},
	} {
		t.Run(asset.path, func(t *testing.T) {
			for _, tc := range []struct {
				name, remote, host, origin string
				want                       int
			}{
				{"local without token", "127.0.0.1:1234", "127.0.0.1:8080", "", 200},
				{"matching origin", "127.0.0.1:1234", "127.0.0.1:8080", "http://127.0.0.1:8080", 200},
				{"LAN", "192.168.1.5:1234", "127.0.0.1:8080", "", 403},
				{"Host", "127.0.0.1:1234", "evil.example", "", 403},
				{"Origin", "127.0.0.1:1234", "127.0.0.1:8080", "http://evil.example", 403},
			} {
				r := httptest.NewRequest("GET", asset.path, nil)
				r.RemoteAddr, r.Host = tc.remote, tc.host
				r.Header.Set("Origin", tc.origin)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != tc.want {
					t.Fatalf("%s status=%d want=%d", tc.name, w.Code, tc.want)
				}
				if w.Code == 200 && (w.Header().Get("Content-Type") != asset.contentType || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Body.String(), asset.marker)) {
					t.Fatalf("invalid asset headers/body: %v %s", w.Header(), w.Body.String())
				}
			}
		})
	}
	for _, path := range []string{"/assets/missing.js", "/assets/%2e%2e/http.go", "/assets/web/index.html"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, loopbackRequest(httptest.NewRequest("GET", path, nil)))
		if w.Code != 404 {
			t.Fatalf("unknown/traversal path %s status=%d", path, w.Code)
		}
	}
}

func TestLocalLogRequiresCSRFAndLoopbackIdentity(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	if err := s.Create(ctx, domain.Task{ID: "logged", Status: domain.StatusFailed, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	body := "<script>untrusted()</script>\nsecret output\n"
	if err := s.PutLog(ctx, "logged", []byte(body)); err != nil {
		t.Fatal(err)
	}
	h := API{Store: s, CSRFToken: testCSRFToken}.Handler()
	for _, tc := range []struct {
		name, remote, host, origin, token string
		want                              int
	}{
		{"local", "127.0.0.1:1234", "127.0.0.1:8080", "http://127.0.0.1:8080", testCSRFToken, 200},
		{"missing token", "127.0.0.1:1234", "127.0.0.1:8080", "", "", 403},
		{"bad token", "127.0.0.1:1234", "127.0.0.1:8080", "", "wrong", 403},
		{"LAN", "192.168.1.5:1234", "127.0.0.1:8080", "", testCSRFToken, 403},
		{"Host", "127.0.0.1:1234", "evil.example", "", testCSRFToken, 403},
		{"Origin", "127.0.0.1:1234", "127.0.0.1:8080", "http://evil.example", testCSRFToken, 403},
	} {
		r := httptest.NewRequest("GET", "/api/v1/tasks/logged/log", nil)
		r.RemoteAddr, r.Host = tc.remote, tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-CSRF-Token", tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s status=%d want=%d", tc.name, w.Code, tc.want)
		}
		if w.Code == 200 && (w.Body.String() != body || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" || w.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatalf("unsafe log response: %v %s", w.Header(), w.Body.String())
		}
		if w.Code != 200 && strings.Contains(w.Body.String(), "secret output") {
			t.Fatal("rejected request leaked log")
		}
	}
}

func TestLocalLogUnavailableMissingDeletedAndExpired(t *testing.T) {
	for _, store := range []Store{nil, NewMemoryStore()} {
		w := httptest.NewRecorder()
		API{Store: store, CSRFToken: testCSRFToken}.Handler().ServeHTTP(w, loopbackRequest(httptest.NewRequest("GET", "/api/v1/tasks/missing/log", nil)))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "storage_unavailable") {
			t.Fatalf("unsupported storage status=%d %s", w.Code, w.Body.String())
		}
	}
	s := authStore(t)
	ctx := context.Background()
	h := API{Store: s, CSRFToken: testCSRFToken}.Handler()
	for _, id := range []string{"missing", "deleted", "expired"} {
		if id != "missing" {
			finished := time.Now().Add(-8 * 24 * time.Hour).UTC()
			if err := s.Create(ctx, domain.Task{ID: id, Status: domain.StatusFailed, CreatedAt: finished}); err != nil {
				t.Fatal(err)
			}
			if err := s.PutLog(ctx, id, []byte("private log")); err != nil {
				t.Fatal(err)
			}
			if id == "deleted" {
				if err := s.Delete(ctx, id); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.db.ExecContext(ctx, "UPDATE tasks SET finished_at=? WHERE id=?", finished.Unix(), id); err != nil {
					t.Fatal(err)
				}
				if _, err := s.DeleteExpired(ctx, time.Now().Add(-7*24*time.Hour), 100); err != nil {
					t.Fatal(err)
				}
			}
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, loopbackRequest(httptest.NewRequest("GET", "/api/v1/tasks/"+id+"/log", nil)))
		if w.Code != 404 || !strings.Contains(w.Body.String(), "log_not_found") {
			t.Fatalf("%s status=%d body=%s", id, w.Code, w.Body.String())
		}
	}
}

func TestLANLogKeepsSessionGateAndPlainTextHeaders(t *testing.T) {
	s := authStore(t)
	ctx := context.Background()
	now := time.Now()
	if err := s.Create(ctx, domain.Task{ID: "logged", Status: domain.StatusFailed, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutLog(ctx, "logged", []byte("private log")); err != nil {
		t.Fatal(err)
	}
	p, code, err := NewPairing(s, now)
	if err != nil {
		t.Fatal(err)
	}
	token, err := p.Consume(code, now)
	if err != nil {
		t.Fatal(err)
	}
	h := NewLANHandler(API{Store: s}, p, []string{"127.0.0.1"})
	read := func(cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/tasks/logged/log", nil)
		r.Host = "127.0.0.1:8080"
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "repotest_read", Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := read(""); w.Code != 401 {
		t.Fatalf("unpaired status=%d", w.Code)
	}
	if w := read(token); w.Code != 200 || w.Body.String() != "private log" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("paired response=%d %v %s", w.Code, w.Header(), w.Body.String())
	}
	if err := p.Revoke(token); err != nil {
		t.Fatal(err)
	}
	if w := read(token); w.Code != 401 {
		t.Fatalf("revoked status=%d", w.Code)
	}
}
