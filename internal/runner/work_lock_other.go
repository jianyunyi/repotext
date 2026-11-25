//go:build !linux

package runner

import (
	"errors"
	"os"
	"path/filepath"
)

type RunnerWorkLock struct{}

func RunnerServiceLockFile() (string, error) {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "repotest", "runner.lock"), nil
}

func dockerManagedLabelValue() string { return "io.repotest.owner=repotest-s4-v1-unsupported" }

func AcquireRunnerWorkLock(string) (*RunnerWorkLock, error) {
	return nil, errors.New("runner work lock is available only on Linux")
}
func (*RunnerWorkLock) Close() error { return nil }
