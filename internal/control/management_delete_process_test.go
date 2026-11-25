//go:build linux

package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"repotest/internal/domain"
	"repotest/internal/runner"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDeletingRunningTaskKillsAndWaitsForLinuxProcessGroup(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "running-process.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task := domain.Task{ID: "running-process", Status: domain.StatusRunning, CreatedAt: time.Now()}
	if err = s.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	registry := NewTaskProcessRegistry()
	runDone := make(chan error, 1)
	go func() {
		runDone <- registry.Run(context.Background(), task.ID, func(ctx context.Context) error {
			_, err := (runner.ProcessRunner{}).Run(ctx, "", nil, "sh", "-c", "sleep 60 & echo $! > \"$1\"; wait", "sh", pidFile)
			return err
		})
	}()
	var child int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, e := os.ReadFile(pidFile)
		if e == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if child > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if child == 0 {
		t.Fatal("child process did not start")
	}
	h := (API{Store: s, Processes: registry, CSRFToken: testCSRFToken}).Handler()
	req := loopbackRequest(httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+task.ID, nil))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d body=%s", res.Code, res.Body.String())
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("task runner did not return after process cancellation")
	}
	if processGroupMemberRunning(child) {
		t.Fatalf("child process %d remained alive after DELETE returned", child)
	}
	if _, err = s.Get(context.Background(), task.ID); err == nil {
		t.Fatal("task persisted after delete")
	}
}

func processGroupMemberRunning(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	fields := strings.Fields(string(b))
	return len(fields) > 2 && fields[2] != "Z" && fields[2] != "X"
}
