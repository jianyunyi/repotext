//go:build linux

package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessRunnerCancellationStopsAndReapsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (ProcessRunner{}).Run(ctx, "", nil, "sh", "-c", "sleep 60 & echo $! > \"$1\"; wait", "sh", pidFile)
		done <- err
	}()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		t.Fatal("child process pid was not recorded")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not wait for process group exit")
	}
	if processRunning(pid) {
		t.Fatalf("child process %d still running after Run returned", pid)
	}
}

func processRunning(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
	}
	fields := strings.Fields(string(b))
	return len(fields) > 2 && fields[2] != "Z" && fields[2] != "X"
}
