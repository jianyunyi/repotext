package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupScratchRemovesOnlyMarkedTaskDirectories(t *testing.T) {
	root := t.TempDir()
	marked := filepath.Join(root, "task-recover-me")
	unmarked := filepath.Join(root, "task-user-data")
	if err := os.Mkdir(marked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(unmarked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marked, taskWorkMarker), []byte("repotest-task-work-v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmarked, "keep.txt"), []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cleanTaskScratch(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marked); !os.IsNotExist(err) {
		t.Fatalf("owned task directory remains: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(unmarked, "keep.txt")); err != nil || string(body) != "unrelated" {
		t.Fatalf("unrelated task data changed: body=%q err=%v", body, err)
	}
}

func TestCleanupScratchRejectsInvalidOwnershipMarker(t *testing.T) {
	root := t.TempDir()
	marked := filepath.Join(root, "task-invalid")
	if err := os.Mkdir(marked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marked, taskWorkMarker), []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cleanTaskScratch(root); err == nil {
		t.Fatal("invalid ownership marker accepted")
	}
	if _, err := os.Stat(marked); err != nil {
		t.Fatalf("invalid marked data was deleted: %v", err)
	}
}
