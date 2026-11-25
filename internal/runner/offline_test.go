package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

func TestPinnedGitHubCommitRunsOffline(t *testing.T) {
	image := os.Getenv("REPOTEST_TEST_IMAGE")
	if runtime.GOOS != "linux" || image == "" || os.Getenv("REPOTEST_TEST_GITHUB") != "1" {
		t.Skip("requires Linux Docker, REPOTEST_TEST_IMAGE, and REPOTEST_TEST_GITHUB=1")
	}
	task := domain.Task{ID: "gorilla-mux-smoke", RepoURL: "https://github.com/gorilla/mux", Commit: "b4617d0b9670ad14039b2739167fd35a60f557c5"}
	source := newArchiveTestDestination(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := NewArchiveFetcher(nil).FetchAndExtract(ctx, task.RepoURL, task.Commit, source); err != nil {
		t.Fatal(err)
	}
	result, err := runOfflineGo(ctx, image, source)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("offline test: exit=%d err=%v output=%s", result.ExitCode, err, result.Output)
	}
	report := offlineReport(task, "go1.27.1", result)
	for _, pkg := range report.Packages {
		if pkg.ImportPath == "github.com/gorilla/mux" && pkg.Status == "passed" {
			t.Logf("commit=%s package=%s status=%s elapsed_ms=%d", task.Commit, pkg.ImportPath, pkg.Status, pkg.ElapsedMS)
			return
		}
	}
	t.Fatalf("no passing mux package: %+v", report)
}

func TestOfflineGoInDocker(t *testing.T) {
	image := os.Getenv("REPOTEST_TEST_IMAGE")
	if runtime.GOOS != "linux" || image == "" {
		t.Skip("requires Linux Docker and REPOTEST_TEST_IMAGE")
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.test/sandbox\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const testSource = `package sandbox
import ("os"; "strings"; "testing"; "time")
func TestSandbox(t *testing.T) {
	if os.Geteuid() == 0 { t.Fatal("running as root") }
	net, err := os.ReadDir("/sys/class/net")
	if err != nil || len(net) != 1 || net[0].Name() != "lo" { t.Fatalf("network: %v %v", net, err) }
	if err := os.WriteFile("/etc/repotest-probe", []byte("probe"), 0600); err == nil { t.Fatal("root filesystem is writable") }
	limits, err := os.ReadFile("/sys/fs/cgroup/pids.max")
	if err != nil || strings.TrimSpace(string(limits)) != "128" { t.Fatalf("pids.max=%q err=%v", limits, err) }
	memory, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil || strings.TrimSpace(string(memory)) != "805306368" { t.Fatalf("memory.max=%q err=%v", memory, err) }
	cpu, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil || strings.TrimSpace(string(cpu)) != "100000 100000" { t.Fatalf("cpu.max=%q err=%v", cpu, err) }
	if strings.Contains(t.Name(), "Slow") { time.Sleep(time.Hour) }
}
`
	setTest := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, "sandbox_test.go"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setTest(testSource)
	result, err := runOfflineGo(context.Background(), image, source)
	if err != nil || result.ExitCode != 0 || !strings.Contains(string(result.Output), `"Action":"pass"`) {
		t.Fatalf("offline test: exit=%d err=%v output=%s", result.ExitCode, err, result.Output)
	}

	setTest(strings.Replace(testSource, "func TestSandbox(", "func TestSandboxSlow(", 1))
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	_, err = runOfflineGo(ctx, image, source)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	remaining, err := exec.Command("docker", "ps", "-aq", "--filter", "label="+dockerManagedLabelValue()).CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(remaining))) != 0 {
		t.Fatalf("offline test containers remain: %s (%v)", remaining, err)
	}
}
