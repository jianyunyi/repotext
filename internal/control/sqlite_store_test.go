package control

import (
	"context"
	"path/filepath"
	"repotest/internal/domain"
	"testing"
	"time"
)

func TestSQLiteStorePersistsTaskAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "repotest.db")
	s, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task-1", RepoURL: "https://github.com/acme/repo", Status: domain.StatusQueued, CreatedAt: time.Now().UTC()}
	if err = s.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != task.RepoURL || got.Status != domain.StatusQueued {
		t.Fatalf("got %+v", got)
	}
}
