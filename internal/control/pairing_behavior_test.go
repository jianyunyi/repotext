package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"repotest/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"
)

func authStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "auth.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestPairingLifecycleAndConcurrency(t *testing.T) {
	s := authStore(t)
	now := time.Now()
	p, code, e := NewPairing(s, now)
	if e != nil {
		t.Fatal(e)
	}
	token, e := p.Consume(code, now)
	if e != nil || !p.Valid(token, now) {
		t.Fatalf("consume/valid: %v", e)
	}
	if _, e = p.Consume(code, now); e == nil {
		t.Fatal("replay accepted")
	}
	if e = p.Revoke(token); e != nil || p.Valid(token, now) {
		t.Fatal("revoke failed")
	}
	expired, ec, e := NewPairing(s, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = expired.Consume(ec, now.Add(6*time.Minute)); e == nil {
		t.Fatal("expired code accepted")
	}
	locked, lc, e := NewPairing(s, now)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5; i++ {
		_, _ = locked.Consume("wrong", now)
	}
	if _, e = locked.Consume(lc, now); e == nil {
		t.Fatal("rate limit failed")
	}
	race, rc, e := NewPairing(s, now)
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, x := race.Consume(rc, now); x == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins != 1 {
		t.Fatalf("concurrent winners=%d", wins)
	}
}
func TestLANPairingReadOnlyRoutes(t *testing.T) {
	s := authStore(t)
	p, code, e := NewPairing(s, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	mem := NewMemoryStore()
	task := domain.Task{ID: "t1", Status: domain.StatusQueued, CreatedAt: time.Now()}
	if e = mem.Create(context.Background(), task); e != nil {
		t.Fatal(e)
	}
	h := NewLANHandler(API{Store: mem}, p, []string{"127.0.0.1"})
	req := httptest.NewRequest("GET", "/api/v1/tasks/t1", nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("unpaired status %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/pair", strings.NewReader(`{"code":"`+code+`"}`))
	req.Host = "127.0.0.1:8080"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("pair status %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe pairing cookie")
	}
	cookie := cookies[0]
	req = httptest.NewRequest("GET", "/api/v1/tasks/t1", nil)
	req.Host = "127.0.0.1:8080"
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("paired read %d", w.Code)
	}
	var got domain.Task
	if e = json.Unmarshal(w.Body.Bytes(), &got); e != nil || got.ID != "t1" {
		t.Fatalf("body %s err %v", w.Body.String(), e)
	}
	for _, tc := range []struct{ m, p string }{{"POST", "/api/v1/previews"}, {"POST", "/api/v1/tasks"}, {"POST", "/api/v1/tasks/t1/cancel"}, {"DELETE", "/api/v1/tasks/t1"}, {"PUT", "/api/v1/tasks/t1"}} {
		req = httptest.NewRequest(tc.m, tc.p, nil)
		req.Host = "127.0.0.1:8080"
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 405 {
			t.Errorf("%s %s got %d", tc.m, tc.p, w.Code)
		}
	}
	req = httptest.NewRequest("GET", "/admin", nil)
	req.Host = "127.0.0.1:8080"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("unknown path %d", w.Code)
	}
}
