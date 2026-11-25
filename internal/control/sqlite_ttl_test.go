package control

import (
	"context"
	"path/filepath"
	"repotest/internal/domain"
	"testing"
	"time"
)

func TestSQLiteDeleteExpiredCascadesReportAndLog(t *testing.T) {
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "ttl.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	finished := time.Now().Add(-8 * 24 * time.Hour).UTC()
	task := domain.Task{ID: "old", Status: domain.StatusFailed, CreatedAt: finished, FinishedAt: &finished}
	if e = s.Create(ctx, task); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.ExecContext(ctx, "UPDATE tasks SET finished_at=? WHERE id=?", finished.Unix(), task.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.PutReport(ctx, task.ID, []byte(`{"ok":true}`)); e != nil {
		t.Fatal(e)
	}
	if e = s.PutLog(ctx, task.ID, []byte("log")); e != nil {
		t.Fatal(e)
	}
	n, e := s.DeleteExpired(ctx, time.Now().Add(-7*24*time.Hour), 100)
	if e != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, e)
	}
	if _, e = s.Get(ctx, task.ID); e == nil {
		t.Fatal("task survived TTL")
	}
	if _, e = s.GetReport(ctx, task.ID); e == nil {
		t.Fatal("report survived cascade")
	}
	if _, e = s.GetLog(ctx, task.ID); e == nil {
		t.Fatal("log survived cascade")
	}
}
