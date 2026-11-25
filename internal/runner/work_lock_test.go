package runner

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunnerWorkLockSerializesEnabledInstances(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("file-lock integration is Linux-specific")
	}
	lockPath := filepath.Join(t.TempDir(), "runner.lock")
	if err := os.Chmod(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireRunnerWorkLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AcquireRunnerWorkLock(lockPath); err == nil {
		t.Fatal("second live runner acquired the same work directory")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireRunnerWorkLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
}

func TestRunnerLockAndDockerOwnerScopeDoNotFollowEnvironmentCacheDirs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("service lock scope is Linux-specific")
	}
	got, err := RunnerServiceLockFile()
	account, userErr := user.Current()
	want := filepath.Join(account.HomeDir, ".local", "state", "repotest", "runner.lock")
	if err != nil || userErr != nil || got != want {
		t.Fatalf("runner service lock file=%q err=%v", got, err)
	}
	if got := dockerManagedLabelValue(); got == "" || got == "io.repotest.owner=repotest-s4-v1" {
		t.Fatalf("Docker cleanup label is not scoped to the current OS user: %q", got)
	}
}
