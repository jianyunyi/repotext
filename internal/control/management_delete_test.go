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

func TestLoopbackAdminDeleteRemovesTerminalTaskAndArtifacts(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task := domain.Task{ID: "done", Status: domain.StatusSucceeded, CreatedAt: time.Now()}
	if err = s.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err = s.PutReport(context.Background(), task.ID, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.PutLog(context.Background(), task.ID, []byte("output")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/done", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "127.0.0.1:8080"
	req.Header.Set("X-CSRF-Token", testCSRFToken)
	res := httptest.NewRecorder()
	(API{Store: s, CSRFToken: testCSRFToken}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err = s.Get(context.Background(), task.ID); err == nil {
		t.Fatal("task remains after delete")
	}
	if _, err = s.GetReport(context.Background(), task.ID); err == nil {
		t.Fatal("report remains after cascade")
	}
	if _, err = s.GetLog(context.Background(), task.ID); err == nil {
		t.Fatal("log remains after cascade")
	}
}

func TestLoopbackAdminDeleteRejectsInvalidOriginAndActiveTasks(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "delete-deny.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, task := range []domain.Task{{ID: "active", Status: domain.StatusRunning, CreatedAt: time.Now()}, {ID: "done", Status: domain.StatusSucceeded, CreatedAt: time.Now()}} {
		if err = s.Create(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	h := (API{Store: s, CSRFToken: testCSRFToken}).Handler()
	cases := []struct {
		name, id, remote, host, origin string
		want                           int
	}{
		{"remote LAN", "done", "192.168.1.8:1234", "127.0.0.1:8080", "", http.StatusForbidden},
		{"spoofed Host", "done", "127.0.0.1:1234", "attacker.example:8080", "", http.StatusForbidden},
		{"localhost Host alias", "done", "127.0.0.1:1234", "localhost:8080", "", http.StatusForbidden},
		{"foreign Origin", "done", "127.0.0.1:1234", "127.0.0.1:8080", "http://attacker.example", http.StatusForbidden},
		{"active", "active", "127.0.0.1:1234", "127.0.0.1:8080", "", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+tc.id, nil)
			req.RemoteAddr = tc.remote
			req.Host = tc.host
			req.Header.Set("X-CSRF-Token", testCSRFToken)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", res.Code, tc.want, res.Body.String())
			}
		})
	}
	if _, err = s.Get(context.Background(), "done"); err != nil {
		t.Fatal("rejected remote request deleted task")
	}
	if _, err = s.Get(context.Background(), "active"); err != nil {
		t.Fatal("active task was deleted")
	}
}

func TestLANHandlerRejectsTaskDeletion(t *testing.T) {
	h := NewLANHandler(API{Store: NewMemoryStore()}, nil, []string{"127.0.0.1"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/done", nil)
	req.Host = "127.0.0.1:8080"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("LAN DELETE status=%d", res.Code)
	}
}

func TestLoopbackCancelRejectsLANAndAcceptsMatchingLocalOrigin(t *testing.T) {
	s := NewMemoryStore()
	task := domain.Task{ID: "queued", Status: domain.StatusQueued, CreatedAt: time.Now()}
	if err := s.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	h := (API{Store: s, CSRFToken: testCSRFToken}).Handler()
	remote := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/queued/cancel", nil)
	remote.RemoteAddr = "192.168.1.5:5000"
	remote.Host = "127.0.0.1:8080"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, remote)
	if res.Code != http.StatusForbidden {
		t.Fatalf("remote cancel status=%d", res.Code)
	}
	got, err := s.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusQueued {
		t.Fatalf("remote request changed task: task=%+v err=%v", got, err)
	}
	local := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/queued/cancel", nil)
	local.RemoteAddr = "127.0.0.1:5000"
	local.Host = "127.0.0.1:8080"
	local.Header.Set("Origin", "http://127.0.0.1:8080")
	local.Header.Set("X-CSRF-Token", testCSRFToken)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, local)
	if res.Code != http.StatusNoContent {
		t.Fatalf("local cancel status=%d body=%s", res.Code, res.Body.String())
	}
	got, err = s.Get(context.Background(), task.ID)
	if err != nil || got.Status != domain.StatusCancelled {
		t.Fatalf("local cancel not applied: task=%+v err=%v", got, err)
	}
}

type blockingTaskProcesses struct {
	started chan struct{}
	release chan struct{}
}

func (p blockingTaskProcesses) CancelAndWait(ctx context.Context, taskID string) error {
	close(p.started)
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestDeleteWaitsForRunningTaskProcessBeforeDeleting(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "delete-running.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task := domain.Task{ID: "running", Status: domain.StatusRunning, CreatedAt: time.Now()}
	if err = s.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err = s.PutReport(context.Background(), task.ID, []byte(`{"old":true}`)); err != nil {
		t.Fatal(err)
	}
	proc := blockingTaskProcesses{started: make(chan struct{}), release: make(chan struct{})}
	h := (API{Store: s, Processes: proc, CSRFToken: testCSRFToken}).Handler()
	req := loopbackRequest(httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/running", nil))
	res := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(res, req); close(done) }()
	<-proc.started
	if _, err = s.Get(context.Background(), task.ID); err != nil {
		t.Fatalf("task deleted before process reaped: %v", err)
	}
	if _, err = s.GetReport(context.Background(), task.ID); err != nil {
		t.Fatalf("report deleted before process reaped: %v", err)
	}
	close(proc.release)
	<-done
	if res.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err = s.Get(context.Background(), task.ID); err == nil {
		t.Fatal("task remains after process reaped and delete completed")
	}
}
