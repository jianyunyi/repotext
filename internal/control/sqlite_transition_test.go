package control

import (
	"context"
	"path/filepath"
	"repotest/internal/domain"
	"testing"
	"time"
)

func TestSQLiteTransitionCannotLoseConcurrentUpdate(t *testing.T) {
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "race.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	task := domain.Task{ID: "x", Status: domain.StatusQueued, CreatedAt: time.Now()}
	if e = s.Create(ctx, task); e != nil {
		t.Fatal(e)
	}
	a, e := s.Transition(ctx, "x", domain.StatusQueued, domain.StatusRunning, "")
	if e != nil || !a {
		t.Fatalf("first %v %v", a, e)
	}
	b, e := s.Transition(ctx, "x", domain.StatusQueued, domain.StatusCancelled, "")
	if e != nil {
		t.Fatal(e)
	}
	if b {
		t.Fatal("stale transition won")
	}
	got, e := s.Get(ctx, "x")
	if e != nil || got.Status != domain.StatusRunning {
		t.Fatalf("got %+v %v", got, e)
	}
}
