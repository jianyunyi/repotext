package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repotest/internal/domain"
)

func readReportRegressionCapture(t *testing.T, id string) ([]byte, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus-capture", id, "output.raw"))
	if err != nil {
		t.Fatal(err)
	}
	return raw, filepath.Join("testdata", "corpus-source", id)
}

func TestReportRealCorpusBuildVariantAndRelativeWindowsLocation(t *testing.T) {
	raw, source := readReportRegressionCapture(t, "build-fail")
	// The corpus is an owned fixture, not a claim that this illustrative GitHub
	// URL/commit supplied to the parser represents a real captured repository.
	task := domain.Task{RepoURL: "https://github.com/acme/corpus", Commit: strings.Repeat("a", 40)}
	report := offlineReportWithSource(task, "go1.26.0", Result{Output: raw, ExitCode: 1}, source)
	if len(report.Packages) != 1 || report.Summary.PackageFailures != 1 {
		t.Errorf("one build failure split across variants: packages=%d summary=%+v", len(report.Packages), report.Summary)
	}
	var canonical *domain.Package
	for i := range report.Packages {
		if report.Packages[i].ImportPath == "example.invalid/repotest-owned-corpus/build-fail" {
			canonical = &report.Packages[i]
		}
	}
	if canonical == nil {
		t.Fatal("canonical package missing")
	}
	if canonical.Status != "build_failed" || !strings.Contains(canonical.Output, "ownedUndefinedSymbol") || !strings.Contains(canonical.Output, `.\source.go:2:179`) {
		t.Errorf("canonical build diagnostics lost: %+v", canonical)
	}
	loc := canonical.FailureLocation
	if loc == nil || loc.Path != "source.go" || loc.Line != 2 || loc.GitHubURL != "https://github.com/acme/corpus/blob/"+task.Commit+"/source.go#L2" {
		t.Errorf("verified relative Windows build location missing: %+v", loc)
	}
	t.Logf("actual raw build-fail: packages=%d package_failures=%d location=%+v", len(report.Packages), report.Summary.PackageFailures, loc)
}

func TestReportRealCorpusNoSelectedTests(t *testing.T) {
	raw, source := readReportRegressionCapture(t, "no-selected-tests")
	report := offlineReportWithSource(domain.Task{}, "go1.26.0", Result{Output: raw}, source)
	if len(report.Packages) != 1 {
		t.Fatalf("package count: %+v", report)
	}
	pkg := report.Packages[0]
	if !report.NoTests || pkg.Status != "no_tests" || pkg.CoveragePct != nil || len(pkg.Tests) != 0 || report.Summary.Passed != 0 {
		t.Fatalf("no tests selected presented as executed: package=%+v report=%+v", pkg, report)
	}
	t.Logf("actual raw no-selected-tests: status=%s coverage=%v no_tests=%v passed=%d", pkg.Status, pkg.CoveragePct, report.NoTests, report.Summary.Passed)
}

func TestReportExternalTestBuildVariantUsesBracketOwner(t *testing.T) {
	output := `{"Action":"build-output","ImportPath":"example.net/lib_test [example.net/lib.test]","Output":"external_test.go:2: undefined: missing\n"}` + "\n" +
		`{"Action":"build-fail","ImportPath":"example.net/lib_test [example.net/lib.test]"}` + "\n" +
		`{"Action":"fail","Package":"example.net/lib","FailedBuild":"example.net/lib_test [example.net/lib.test]"}` + "\n"
	report := offlineReport(domain.Task{}, "", Result{Output: []byte(output), ExitCode: 1})
	if len(report.Packages) != 1 || report.Packages[0].ImportPath != "example.net/lib" || report.Summary.PackageFailures != 1 || !strings.Contains(report.Packages[0].Output, "undefined") {
		t.Fatalf("external test variant assigned to literal prefix: %+v", report)
	}
}

func TestReportRelativeWindowsSourceMappingAndAbsoluteRejection(t *testing.T) {
	source := reportSourceFixture(t)
	for _, tc := range []struct{ filename, pkg, want string }{
		{`.\root_test.go`, "example.net/alternate/v2", "root_test.go"},
		{`sub dir\case_test.go`, "example.net/alternate/v2", "sub dir/case_test.go"},
		{`C:\root_test.go`, "example.net/alternate/v2", ""},
		{`C:root_test.go`, "example.net/alternate/v2", ""},
		{`\\server\root_test.go`, "example.net/alternate/v2", ""},
		{`\root_test.go`, "example.net/alternate/v2", ""},
		{`..\root_test.go`, "example.net/alternate/v2", ""},
		{`.\sub dir\..\root_test.go`, "example.net/alternate/v2", ""},
		{`sub dir\missing.go`, "example.net/alternate/v2", ""},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			output := eventLine("output", tc.pkg, "TestFail", tc.filename+":2: bad\n") + eventLine("fail", tc.pkg, "TestFail", "")
			report := offlineReportWithSource(domain.Task{}, "", Result{Output: []byte(output)}, source)
			loc := report.Packages[0].Tests[0].FailureLocation
			if tc.want == "" {
				if loc != nil {
					t.Fatalf("absolute/unsafe Windows diagnostic mapped: %+v", loc)
				}
				return
			}
			if loc == nil || loc.Path != tc.want || loc.Line != 2 {
				t.Fatalf("safe Windows relative source unmapped: %+v", loc)
			}
		})
	}
}

func TestReportNoSelectedSummaryDoesNotOverrideObservedSkipCase(t *testing.T) {
	output := eventLine("run", "m", "TestSkip", "") + eventLine("skip", "m", "TestSkip", "") + eventLine("output", "m", "", "coverage: 0.0% of statements\nok  \tm\t0.1s\tcoverage: 0.0% of statements [no tests to run]\n") + eventLine("pass", "m", "", "")
	report := offlineReport(domain.Task{}, "", Result{Output: []byte(output)})
	if report.NoTests || report.Packages[0].Status != "passed" || report.Packages[0].CoveragePct == nil || *report.Packages[0].CoveragePct != 0 || report.Summary.Skipped != 1 {
		t.Fatalf("observed skip rewritten as no tests: %+v", report)
	}
}

func TestReportActualTestMainPanicBeforeRunClearsCoverage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "quality-probe", "output.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	report := offlineReportWithSource(domain.Task{}, "go1.26.0", Result{Output: raw, ExitCode: 1}, filepath.Join("testdata", "quality-probe"))
	if len(report.Packages) != 1 {
		t.Fatalf("packages: %+v", report)
	}
	pkg := report.Packages[0]
	if pkg.Status != "failed" || len(pkg.Tests) != 0 || pkg.CoveragePct != nil || !strings.Contains(pkg.Output, "before m.Run") || !strings.Contains(pkg.Output, "99.9%") {
		t.Fatalf("pre-execution TestMain panic supplied measured coverage: %+v", pkg)
	}
	t.Logf("actual TestMain pre-run panic: status=%s cases=%d coverage=%v diagnostic_retained=%v", pkg.Status, len(pkg.Tests), pkg.CoveragePct, strings.Contains(pkg.Output, "before m.Run"))
}

func TestReportActualCompletedAssertionFailureKeepsCoverage(t *testing.T) {
	raw, source := readReportRegressionCapture(t, "assertion-fail")
	report := offlineReportWithSource(domain.Task{}, "go1.26.0", Result{Output: raw, ExitCode: 1}, source)
	if len(report.Packages) != 1 || report.Packages[0].CoveragePct == nil || *report.Packages[0].CoveragePct != 100 || report.Summary.Failed != 1 {
		t.Fatalf("completed failed test lost coverage: %+v", report)
	}
	t.Logf("actual completed assertion failure: failed=%d coverage=%v", report.Summary.Failed, *report.Packages[0].CoveragePct)
}
