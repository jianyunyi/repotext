package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"repotest/internal/domain"
)

func reportWindowsShortPath(t *testing.T, root string) string {
	t.Helper()
	pointer, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 32768)
	n, err := syscall.GetShortPathName(pointer, &buffer[0], uint32(len(buffer)))
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 || int(n) >= len(buffer) {
		t.Fatalf("short path length=%d", n)
	}
	short := syscall.UTF16ToString(buffer[:n])
	if strings.EqualFold(root, short) {
		t.Skip("volume has no 8.3 alias for source fixture")
	}
	return short
}

func logReportWindowsRoot(t *testing.T, root string) {
	t.Helper()
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("root=%q resolved=%q root_mode=%s", absolute, resolved, info.Mode())
	for ancestor := absolute; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Logf("redirecting ancestor=%q mode=%s", ancestor, info.Mode())
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
}

func assertReportWindowsRootMapsSource(t *testing.T, root string) {
	t.Helper()
	logReportWindowsRoot(t, root)
	output := eventLine("output", "example.net/alternate/v2", "TestFail", `root_test.go:2: wrong`+"\n") + eventLine("fail", "example.net/alternate/v2", "TestFail", "")
	report := offlineReportWithSource(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40)}, "", Result{Output: []byte(output)}, root)
	loc := report.Packages[0].Tests[0].FailureLocation
	if loc == nil || loc.Path != "root_test.go" || loc.Line != 2 || loc.GitHubURL == "" {
		t.Fatalf("ordinary Windows source alias lost trusted mapping: %+v", loc)
	}
}

func TestReportSourceWindowsShortPathIsRegularDirectory(t *testing.T) {
	root := reportSourceFixture(t)
	short := reportWindowsShortPath(t, root)
	assertReportWindowsRootMapsSource(t, short)
}

func TestReportSourceWindowsCaseAliasIsRegularDirectory(t *testing.T) {
	root := reportSourceFixture(t)
	assertReportWindowsRootMapsSource(t, strings.ToLower(root))
}

func TestReportSourceWindowsJunctionAncestorRejected(t *testing.T) {
	root := reportSourceFixture(t)
	alias := filepath.Join(t.TempDir(), "junction")
	output, err := exec.Command("cmd", "/c", "mklink", "/J", alias, filepath.Dir(root)).CombinedOutput()
	if err != nil {
		t.Skipf("junction creation unavailable: %v %s", err, output)
	}
	t.Logf("junction command output=%q alias=%q target=%q", output, alias, filepath.Dir(root))
	info, err := os.Lstat(alias)
	if err != nil {
		t.Fatalf("junction creation returned success without alias: %v", err)
	}
	t.Logf("junction mode=%s", info.Mode())
	t.Cleanup(func() {
		if err := os.Remove(alias); err != nil {
			t.Errorf("remove fixture junction: %v", err)
		}
	})
	sourceAlias := filepath.Join(alias, filepath.Base(root))
	if info, err := os.Stat(sourceAlias); err != nil || !info.IsDir() {
		t.Fatalf("junction does not lead to actual source root: info=%v err=%v", info, err)
	}
	resolved, evalErr := filepath.EvalSymlinks(sourceAlias)
	t.Logf("junction source root=%q resolved=%q eval_error=%v", sourceAlias, resolved, evalErr)
	if source := indexReportSource(sourceAlias, domain.Task{}); source != nil {
		t.Fatal("junction ancestor accepted as validated source")
	}
	if source := indexReportSource(root, domain.Task{}); source == nil {
		t.Fatal("junction fixture damaged ordinary source root")
	}
}
