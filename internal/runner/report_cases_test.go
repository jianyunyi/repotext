package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"repotest/internal/domain"
)

type decodedCase struct {
	Name            string `json:"name"`
	Parent          string `json:"parent"`
	Status          string `json:"status"`
	ElapsedMS       int64  `json:"elapsed_ms"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated"`
	FailureLocation *struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		GitHubURL string `json:"github_url"`
	} `json:"failure_location"`
}
type decodedReport struct {
	RepoURL    string `json:"repo_url"`
	Commit     string `json:"commit"`
	Truncated  bool   `json:"truncated"`
	Incomplete bool   `json:"incomplete"`
	Output     string `json:"output"`
	Summary    struct {
		Passed, Failed, Skipped, Incomplete int
		PackageFailures                     int `json:"package_failures"`
		DroppedCases                        int `json:"dropped_cases"`
	} `json:"summary"`
	Packages []struct {
		ImportPath      string        `json:"import_path"`
		Status          string        `json:"status"`
		Coverage        *float64      `json:"coverage_pct"`
		Output          string        `json:"output"`
		OutputTruncated bool          `json:"output_truncated"`
		Tests           []decodedCase `json:"tests"`
	} `json:"packages"`
}

func decodeReport(t *testing.T, report domain.Report) decodedReport {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got decodedReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}
func eventLine(action, pkg, test, output string) string {
	data, _ := json.Marshal(struct{ Action, Package, Test, Output string }{action, pkg, test, output})
	return string(data) + "\n"
}
func TestReportCaseNestedParallelAndLeafSummary(t *testing.T) {
	output := eventLine("run", "m/p", "TestParent", "") +
		eventLine("run", "m/p", "TestParent/pass", "") + eventLine("pause", "m/p", "TestParent/pass", "") +
		eventLine("run", "m/p", "TestParent/fail", "") + eventLine("cont", "m/p", "TestParent/pass", "") +
		eventLine("output", "m/p", "TestParent/fail", "    test.go:12: wrong\n") +
		eventLine("fail", "m/p", "TestParent/fail", "") +
		`{"Action":"pass","Package":"m/p","Test":"TestParent/pass","Elapsed":0.125}` + "\n" +
		eventLine("fail", "m/p", "TestParent", "") + eventLine("skip", "m/q", "TestSkip", "") +
		eventLine("fail", "m/p", "", "") + eventLine("pass", "m/q", "", "")
	got := decodeReport(t, offlineReport(domain.Task{RepoURL: "https://github.com/a/b", Commit: strings.Repeat("a", 40)}, "go1.23", Result{Output: []byte(output), ExitCode: 1}))
	if len(got.Packages) != 2 || len(got.Packages[0].Tests) != 3 {
		t.Fatalf("case events were lost: %+v", got)
	}
	if got.Packages[0].Tests[0].Name != "TestParent/fail" || got.Packages[0].Tests[0].Parent != "TestParent" || !strings.Contains(got.Packages[0].Tests[0].Output, "wrong") {
		t.Fatalf("failed leaf first: %+v", got.Packages[0].Tests)
	}
	if got.Summary.Passed != 1 || got.Summary.Failed != 1 || got.Summary.Skipped != 1 || got.Summary.PackageFailures != 1 {
		t.Fatalf("leaf summary: %+v", got.Summary)
	}
	if got.RepoURL == "" || got.Commit == "" {
		t.Fatalf("source identity missing: %+v", got)
	}
	for _, tc := range got.Packages[0].Tests {
		if tc.Name == "TestParent/pass" && tc.ElapsedMS != 125 {
			t.Fatalf("elapsed: %+v", tc)
		}
	}
}
func TestReportCaseMissingTerminalPanicAndLateEvents(t *testing.T) {
	output := eventLine("run", "m", "TestPartial", "") + eventLine("pause", "m", "TestPartial", "") +
		eventLine("pass", "m", "", "") + eventLine("output", "m", "TestPartial", "panic: broken\n") +
		eventLine("pass", "m", "TestLate", "") + eventLine("run", "m", "TestLate", "")
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output)}))
	if len(got.Packages[0].Tests) != 2 {
		t.Fatalf("missing cases: %+v", got)
	}
	if got.Summary.Incomplete != 1 || got.Summary.Passed != 1 || !got.Incomplete {
		t.Fatalf("package pass invented terminal: %+v", got)
	}
	for _, tc := range got.Packages[0].Tests {
		if tc.Name == "TestPartial" && (tc.Status != "incomplete" || !strings.Contains(tc.Output, "panic:")) {
			t.Fatalf("panic lost: %+v", tc)
		}
	}
}
func TestReportCaseBuildDiagnosticsAndMixedLines(t *testing.T) {
	output := "# m/bad\n./broken.go:4: undefined: missing\n" + eventLine("output", "m/bad", "", "FAIL\tm/bad [build failed]\n") + eventLine("fail", "m/bad", "", "") + "go: dependency failed\n" +
		`{"Action":"build-output","ImportPath":"m/new","Output":"./new.go:3: invalid\n"}` + "\n" + `{"Action":"build-fail","ImportPath":"m/new"}` + "\n"
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output), ExitCode: 1}))
	if len(got.Packages) != 2 || got.Packages[0].Status != "build_failed" || !strings.Contains(got.Packages[0].Output, "undefined") || !strings.Contains(got.Packages[1].Output, "invalid") {
		t.Fatalf("build diagnostics lost: %+v", got)
	}
	if !strings.Contains(got.Output, "dependency failed") || got.Summary.PackageFailures != 2 {
		t.Fatalf("mixed diagnostics lost: %+v", got)
	}
}
func TestReportCaseCoverageOnlyPackageToolSummary(t *testing.T) {
	output := eventLine("output", "m", "TestSpoof", "coverage: 99.9% of statements\n") + eventLine("pass", "m", "TestSpoof", "") +
		eventLine("output", "m", "", "coverage: 25.0% of statements\n") + eventLine("pass", "m", "", "") +
		eventLine("output", "empty", "", "?\tempty\t[no test files]\n") + eventLine("skip", "empty", "", "") +
		eventLine("run", "zero", "TestZero", "") + eventLine("pass", "zero", "TestZero", "") +
		eventLine("output", "zero", "", "coverage: 0.0% of statements\n") + eventLine("pass", "zero", "", "") +
		eventLine("output", "none", "", "coverage: [no statements]\n") + eventLine("pass", "none", "", "")
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output)}))
	if got.Packages[0].Coverage == nil || *got.Packages[0].Coverage != 25 || got.Packages[1].Coverage != nil || got.Packages[2].Coverage == nil || *got.Packages[2].Coverage != 0 || got.Packages[3].Coverage != nil {
		t.Fatalf("coverage semantics: %+v", got.Packages)
	}
	spoof := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(eventLine("output", "m", "TestSpoof", "coverage: 99.9% of statements\n"))}))
	if spoof.Packages[0].Coverage != nil {
		t.Fatal("test output spoofed package coverage")
	}
}
func TestReportCaseOutputAndCountBudgets(t *testing.T) {
	var output strings.Builder
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("Test%d", i)
		output.WriteString(eventLine("output", "m", name, strings.Repeat("x", 20<<10)))
		output.WriteString(eventLine("pass", "m", name, ""))
	}
	output.WriteString(eventLine("output", "m", "", strings.Repeat("d", 40<<10)))
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output.String())}))
	if !got.Truncated || !got.Incomplete || len(got.Packages[0].Tests) != 40 {
		t.Fatalf("output budget flags: %+v", got)
	}
	total := len(got.Output)
	for _, pkg := range got.Packages {
		if len(pkg.Output) > 32<<10 || !pkg.OutputTruncated {
			t.Fatalf("package bound: %d", len(pkg.Output))
		}
		total += len(pkg.Output)
		for _, tc := range pkg.Tests {
			if len(tc.Output) > 16<<10 || !tc.OutputTruncated {
				t.Fatalf("case bound: %d", len(tc.Output))
			}
			total += len(tc.Output)
		}
	}
	if total > 512<<10 {
		t.Fatalf("aggregate report output=%d", total)
	}
	output.Reset()
	for i := 0; i < 4100; i++ {
		output.WriteString(eventLine("pass", "m", fmt.Sprintf("Test%d", i), ""))
	}
	got = decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output.String())}))
	if len(got.Packages[0].Tests) > 4096 || !got.Truncated || !got.Incomplete || got.Summary.DroppedCases != 4 || got.Summary.Passed != 4096 {
		t.Fatalf("case budget: retained=%d summary=%+v", len(got.Packages[0].Tests), got.Summary)
	}
}
func TestReportCaseRawBudgetAndSerializedSize(t *testing.T) {
	output := eventLine("run", "m", "TestPartial", "") + strings.Repeat("a", (2<<20)+1) + "\n" + eventLine("pass", "m", "TestPartial", "")
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output)}))
	if !got.Truncated || !got.Incomplete || got.Summary.Incomplete != 1 {
		t.Fatalf("raw budget fabricated completion: %+v", got)
	}
	var noisy strings.Builder
	for i := 0; i < 200; i++ {
		noisy.WriteString(eventLine("pass", fmt.Sprintf("m/%d", i), strings.Repeat("n", 10000)+fmt.Sprint(i), ""))
	}
	report := offlineReport(domain.Task{}, "", Result{Output: []byte(noisy.String())})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 8<<20 {
		t.Fatalf("serialized report too large: %d", len(data))
	}
}
func TestOfflineCommandRequestsCoverage(t *testing.T) {
	if !strings.Contains(offlineCommand, "go test -json -cover ") {
		t.Fatalf("coverage not requested: %s", offlineCommand)
	}
}

func TestReportDroppedChildDoesNotCountParentAsPassedLeaf(t *testing.T) {
	var output strings.Builder
	output.WriteString(eventLine("pass", "m", "TestParent", ""))
	for i := 0; i < 4095; i++ {
		output.WriteString(eventLine("pass", "m", fmt.Sprintf("Test%d", i), ""))
	}
	output.WriteString(eventLine("fail", "m", "TestParent/omitted", ""))
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output.String())}))
	if got.Summary.Passed != 4095 || got.Summary.DroppedCases != 1 {
		t.Fatalf("dropped child became passed leaf: %+v", got.Summary)
	}
}

func TestReportNoPackagesWarningAndEmptyUnknownOutput(t *testing.T) {
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte("go: warning: \"./...\" matched no packages\n")}))
	if !strings.Contains(got.Output, "matched no packages") || got.Incomplete {
		t.Fatalf("no-package diagnostic: %+v", got)
	}
	report := offlineReport(domain.Task{}, "", Result{Output: []byte("go: warning: \"./...\" matched no packages\n")})
	if !report.NoTests || report.Summary.Passed != 0 || len(report.Packages) != 0 {
		t.Fatalf("empty module invented tests: %+v", report)
	}
	unknown := offlineReport(domain.Task{}, "", Result{})
	if !unknown.Incomplete || unknown.NoTests {
		t.Fatalf("empty output treated as complete: %+v", unknown)
	}
}

func TestReportNoStatementsClearsCoverage(t *testing.T) {
	output := eventLine("output", "m", "", "coverage: 99.0% of statements\n") + eventLine("output", "m", "", "coverage: [no statements]\n") + eventLine("pass", "m", "", "")
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output)}))
	if got.Packages[0].Coverage != nil {
		t.Fatal("no statements retained a fabricated percentage")
	}
}

func TestReportLeafSummaryWithAdjacentPunctuationNames(t *testing.T) {
	output := eventLine("pass", "m", "TestParent/a", "") + eventLine("pass", "m", "TestParent/a-", "") + eventLine("fail", "m", "TestParent/a/deep", "")
	got := decodeReport(t, offlineReport(domain.Task{}, "", Result{Output: []byte(output)}))
	if got.Summary.Passed != 1 || got.Summary.Failed != 1 {
		t.Fatalf("nonleaf counted through punctuation sibling: %+v", got.Summary)
	}
}

func TestReportCoverNoTestFilesToolSummary(t *testing.T) {
	// Shape observed in the Go-produced no-tests/output.jsonl corpus: -cover emits
	// a package-prefixed summary and pass rather than the older ?/skip sequence.
	output := eventLine("start", "example.test/no-tests", "", "") +
		eventLine("output", "example.test/no-tests", "", "\texample.test/no-tests\t\tcoverage: 0.0% of statements\n") +
		eventLine("pass", "example.test/no-tests", "", "")
	report := offlineReport(domain.Task{}, "", Result{Output: []byte(output)})
	if !report.NoTests || report.Packages[0].Status != "no_tests" || report.Packages[0].CoveragePct != nil || report.Summary.Passed != 0 {
		t.Fatalf("-cover package without tests: %+v", report)
	}
}
