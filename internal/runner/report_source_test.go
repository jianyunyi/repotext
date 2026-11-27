package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repotest/internal/domain"
)

func reportSourceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod":               "module example.net/alternate/v2\n\ngo 1.23\n",
		"root_test.go":         "package root\n// two\n// three\n// four\n",
		"sub dir/case_test.go": "package sub\n// two\n// three\n// four\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReportSourceVerifiedPathsAndFixedCommit(t *testing.T) {
	root := reportSourceFixture(t)
	task := domain.Task{RepoURL: "https://github.com/acme/repo.git", Commit: strings.Repeat("a", 40)}
	for _, path := range []string{"case_test.go", "./case_test.go", "/work/src/sub dir/case_test.go", "example.net/alternate/v2/sub dir/case_test.go"} {
		t.Run(path, func(t *testing.T) {
			output := eventLine("output", "example.net/alternate/v2/sub dir", "TestFail", "    "+path+":3: wrong\n") + eventLine("fail", "example.net/alternate/v2/sub dir", "TestFail", "")
			got := decodeReport(t, offlineReportWithSource(task, "", Result{Output: []byte(output)}, root))
			loc := got.Packages[0].Tests[0].FailureLocation
			if loc == nil || loc.Path != "sub dir/case_test.go" || loc.Line != 3 || loc.GitHubURL != "https://github.com/acme/repo/blob/"+task.Commit+"/sub%20dir/case_test.go#L3" {
				t.Fatalf("verified mapping: %+v", loc)
			}
		})
	}
}
func TestReportSourcePanicSelectsRepositoryStackFrame(t *testing.T) {
	root := reportSourceFixture(t)
	output := eventLine("output", "example.net/alternate/v2", "TestPanic", "panic: broken\n\t/usr/local/go/src/runtime/panic.go:100 +0x1\n\t/work/modules/evil.test/mod@v1/file.go:2 +0x1\n\t/work/src/root_test.go:4 +0x1\n") + eventLine("fail", "example.net/alternate/v2", "TestPanic", "")
	got := decodeReport(t, offlineReportWithSource(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("b", 40)}, "", Result{Output: []byte(output)}, root))
	loc := got.Packages[0].Tests[0].FailureLocation
	if loc == nil || loc.Path != "root_test.go" || loc.Line != 4 {
		t.Fatalf("panic source location: %+v", loc)
	}
}
func TestReportSourceRejectsUnsafeMissingAndUnmappedPaths(t *testing.T) {
	root := reportSourceFixture(t)
	for _, path := range []string{"../root_test.go", "/work/src/sub dir/../root_test.go", "/etc/root_test.go", `C:\secret\root_test.go`, "javascript:root_test.go", "https://evil/root_test.go", "missing.go", "/work/modules/m/root_test.go", "root_test.go:999"} {
		t.Run(path, func(t *testing.T) {
			output := eventLine("output", "example.net/alternate/v2", "TestFail", path+":3: wrong\n") + eventLine("fail", "example.net/alternate/v2", "TestFail", "")
			got := decodeReport(t, offlineReportWithSource(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40)}, "", Result{Output: []byte(output)}, root))
			if got.Packages[0].Tests[0].FailureLocation != nil {
				t.Fatalf("unsafe mapping: %+v", got.Packages[0].Tests[0].FailureLocation)
			}
		})
	}
	output := eventLine("output", "example.net/alternate/v2", "TestFail", "root_test.go:2: wrong\n") + eventLine("fail", "example.net/alternate/v2", "TestFail", "")
	got := decodeReport(t, offlineReport(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40)}, "", Result{Output: []byte(output)}))
	if got.Packages[0].Tests[0].FailureLocation != nil {
		t.Fatal("guessed link without source mapping")
	}
	got = decodeReport(t, offlineReportWithSource(domain.Task{RepoURL: "javascript:alert(1)", Commit: "main"}, "", Result{Output: []byte(output)}, root))
	if got.Packages[0].Tests[0].FailureLocation == nil || got.Packages[0].Tests[0].FailureLocation.GitHubURL != "" {
		t.Fatal("unvalidated URL or ref linked")
	}
}
func TestReportSourceBuildFailureLocation(t *testing.T) {
	root := reportSourceFixture(t)
	output := "# example.net/alternate/v2\n./root_test.go:2: undefined: missing\n" + eventLine("fail", "example.net/alternate/v2", "", "")
	got := offlineReportWithSource(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40)}, "", Result{Output: []byte(output), ExitCode: 1}, root)
	if got.Packages[0].FailureLocation == nil || got.Packages[0].FailureLocation.Path != "root_test.go" {
		t.Fatalf("package build location lost: %+v", got.Packages[0])
	}
}

func TestReportSourceRejectsTraversalInPackagePath(t *testing.T) {
	root := reportSourceFixture(t)
	output := eventLine("output", "example.net/alternate/v2/sub/..", "TestFail", "root_test.go:2: wrong\n") + eventLine("fail", "example.net/alternate/v2/sub/..", "TestFail", "")
	got := decodeReport(t, offlineReportWithSource(domain.Task{RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40)}, "", Result{Output: []byte(output)}, root))
	if got.Packages[0].Tests[0].FailureLocation != nil {
		t.Fatalf("package traversal linked: %+v", got.Packages[0].Tests[0].FailureLocation)
	}
}

func TestReportSourceSymlinkAncestorRejected(t *testing.T) {
	root := reportSourceFixture(t)
	alias := filepath.Join(t.TempDir(), "source-parent-link")
	if err := os.Symlink(filepath.Dir(root), alias); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(alias); err != nil {
			t.Errorf("remove fixture symlink: %v", err)
		}
	})
	if source := indexReportSource(filepath.Join(alias, filepath.Base(root)), domain.Task{}); source != nil {
		t.Fatal("symlink ancestor accepted as validated source")
	}
	if source := indexReportSource(root, domain.Task{}); source == nil {
		t.Fatal("symlink fixture damaged ordinary source root")
	}
}
