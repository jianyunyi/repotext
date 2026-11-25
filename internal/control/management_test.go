package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"repotest/internal/domain"
	"testing"
	"time"
)

func TestLANReadReportAndLogOnlyWithSession(t *testing.T) {
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "lanread.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	task := domain.Task{ID: "r1", Status: domain.StatusSucceeded, CreatedAt: time.Now()}
	if e = s.Create(context.Background(), task); e != nil {
		t.Fatal(e)
	}
	_ = s.PutReport(context.Background(), task.ID, []byte(`{"result":"ok"}`))
	_ = s.PutLog(context.Background(), task.ID, []byte("done"))
	p, code, e := NewPairing(s, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	token, e := p.Consume(code, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	h := NewLANHandler(API{Store: s}, p, []string{"127.0.0.1"})
	for _, path := range []string{"/api/v1/tasks/r1/report", "/api/v1/tasks/r1/log"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "127.0.0.1:8080"
		req.AddCookie(&http.Cookie{Name: "repotest_read", Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Errorf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
