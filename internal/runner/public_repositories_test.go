package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

// This opt-in check executes only the ten pinned, reviewed repositories. It
// preserves the consumer bytes and reports, without running an HTTP service.
func TestPublicRepositoryReportCorpus(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("REPOTEST_PUBLIC_CORPUS") != "1" {
		t.Skip("requires explicitly enabled controlled Linux corpus validation")
	}
	image, out := os.Getenv("REPOTEST_TEST_IMAGE"), os.Getenv("REPOTEST_CORPUS_OUT")
	if !filepath.IsAbs(out) || !imageID.MatchString(image) {
		t.Fatal("requires absolute evidence directory and immutable image")
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	version, err := InspectGoImage(ctx, image)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/public-repositories.json")
	if err != nil {
		t.Fatal(err)
	}
	var repos []struct {
		RepoURL string `json:"repo_url"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(data, &repos); err != nil || len(repos) != 10 {
		t.Fatalf("pinned manifest: count=%d err=%v", len(repos), err)
	}
	writeCorpusJSON(t, filepath.Join(out, "environment.json"), map[string]any{
		"kind": "unmodified_pinned_public_repository_execution", "image_id": image,
		"image_go_version": version, "host_goos": runtime.GOOS, "host_goarch": runtime.GOARCH,
		"offline_command": offlineCommand, "concurrency": 1, "per_repository_deadline_seconds": 300,
		"source_manifest_sha256": corpusSHA(data), "fault_injection": false,
	})
	seen := make(map[string]bool)
	for i, repo := range repos {
		if seen[repo.RepoURL] {
			t.Fatal("duplicate repository")
		}
		seen[repo.RepoURL] = true
		t.Run(strings.TrimPrefix(repo.RepoURL, "https://github.com/"), func(t *testing.T) {
			id := fmt.Sprintf("repo-%02d", i+1)
			dir := filepath.Join(out, id)
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			work := t.TempDir()
			if err := os.Chmod(work, 0o700); err != nil {
				t.Fatal(err)
			}
			executor := DockerExecutor{ImageID: image, GoVersion: version, WorkDir: work}
			started := time.Now().UTC()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			phases := []domain.Phase{}
			execution, runErr := executor.Execute(ctx, domain.Task{ID: id, RepoURL: repo.RepoURL, Commit: repo.Commit}, func(phase domain.Phase) error {
				phases = append(phases, phase)
				return nil
			})
			cancel()
			if err := os.WriteFile(filepath.Join(dir, "output.raw"), execution.Output, 0o600); err != nil {
				t.Fatal(err)
			}
			reportBytes := writeCorpusJSON(t, filepath.Join(dir, "report.json"), execution.Report)
			errText := ""
			if runErr != nil {
				errText = runErr.Error()
			}
			writeCorpusJSON(t, filepath.Join(dir, "provenance.json"), map[string]any{
				"repo_url": repo.RepoURL, "commit": repo.Commit, "started_at": started,
				"finished_at": time.Now().UTC(), "exit_code": execution.ExitCode, "error": errText,
				"timed_out": execution.TimedOut, "raw_truncated": execution.Truncated,
				"raw_sha256": corpusSHA(execution.Output), "report_sha256": corpusSHA(reportBytes),
				"phases": phases, "fault_injection": false, "source_modified": false,
			})
			if runErr != nil {
				t.Fatalf("execution unavailable: %v (retained diagnostics)", runErr)
			}
			if execution.Report.Commit != repo.Commit || execution.Report.RepoURL != repo.RepoURL {
				t.Fatal("report provenance differs from pinned task")
			}
			verifyPublicCorpusReport(t, execution.Output, execution.Report)
			t.Logf("commit=%s image_go=%s exit=%d packages=%d summary=%+v raw_sha256=%s", repo.Commit, version, execution.ExitCode, len(execution.Report.Packages), execution.Report.Summary, corpusSHA(execution.Output))
		})
	}
}

// Independent event reduction checks case terminals and tool package coverage;
// it does not use the production parser's event/state helpers.
func verifyPublicCorpusReport(t *testing.T, raw []byte, report domain.Report) {
	t.Helper()
	if report.Summary == nil || len(report.Packages) == 0 {
		t.Fatal("no structured public repository report")
	}
	cases := make(map[string]map[string]string)
	running := make(map[string]map[string]bool)
	coverage := make(map[string]float64)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var event struct{ Action, Package, Test, Output string }
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Package == "" {
			continue
		}
		if event.Test != "" && event.Action == "run" {
			if running[event.Package] == nil {
				running[event.Package] = make(map[string]bool)
			}
			running[event.Package][event.Test] = true
		}
		if event.Test != "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			if cases[event.Package] == nil {
				cases[event.Package] = make(map[string]string)
			}
			cases[event.Package][event.Test] = event.Action
		}
		if event.Test == "" && event.Action == "output" {
			text := strings.TrimSpace(event.Output)
			if strings.HasPrefix(text, "coverage: ") && strings.HasSuffix(text, "% of statements") {
				if value, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(text, "coverage: "), "% of statements"), 64); err == nil {
					coverage[event.Package] = value
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	reportedPackages := make(map[string]bool)
	for _, pkg := range report.Packages {
		reportedPackages[pkg.ImportPath] = true
		observed := make(map[string]bool)
		for _, test := range pkg.Tests {
			observed[test.Name] = true
			want := map[string]string{"pass": "passed", "fail": "failed", "skip": "skipped"}[cases[pkg.ImportPath][test.Name]]
			if want != "" && test.Status != want {
				t.Errorf("%s/%s status=%s want=%s", pkg.ImportPath, test.Name, test.Status, want)
			}
			if running[pkg.ImportPath][test.Name] && cases[pkg.ImportPath][test.Name] == "" && test.Status != "incomplete" {
				t.Errorf("case without captured terminal falsely ended: %s/%s=%s", pkg.ImportPath, test.Name, test.Status)
			}
			if test.Status == "failed" {
				leaf := true
				for other := range cases[pkg.ImportPath] {
					if strings.HasPrefix(other, test.Name+"/") {
						leaf = false
						break
					}
				}
				if leaf && test.FailureLocation == nil {
					t.Errorf("failed leaf lacks source location: %s/%s", pkg.ImportPath, test.Name)
				}
			}
			if test.FailureLocation != nil && test.FailureLocation.GitHubURL != "" {
				prefix := report.RepoURL + "/blob/" + report.Commit + "/"
				if !strings.HasPrefix(test.FailureLocation.GitHubURL, prefix) || test.FailureLocation.Line < 1 {
					t.Error("source link lost pinned repository boundary")
				}
			}
		}
		if !report.Incomplete {
			for name, status := range cases[pkg.ImportPath] {
				if !observed[name] {
					t.Errorf("dropped observed case %s/%s", pkg.ImportPath, name)
				}
				parent := false
				for other := range cases[pkg.ImportPath] {
					if strings.HasPrefix(other, name+"/") {
						parent = true
						break
					}
				}
				if !parent {
					counts[status]++
				}
			}
		}
		if value, ok := coverage[pkg.ImportPath]; ok && pkg.Status != "no_tests" {
			if pkg.CoveragePct == nil || math.Abs(*pkg.CoveragePct-value) > 0.001 {
				t.Errorf("%s package coverage differs from captured tool summary", pkg.ImportPath)
			}
		}
	}
	for pkg, tests := range running {
		for name := range tests {
			if cases[pkg][name] == "" && !report.Incomplete {
				t.Errorf("captured case without terminal lacks incomplete flag: %s/%s", pkg, name)
			}
		}
	}
	if !report.Incomplete {
		for pkg := range cases {
			if !reportedPackages[pkg] {
				t.Errorf("missing package with captured cases: %s", pkg)
			}
		}
	}
	if !report.Incomplete && (report.Summary.Passed != counts["pass"] || report.Summary.Failed != counts["fail"] || report.Summary.Skipped != counts["skip"]) {
		t.Errorf("leaf summary differs from independent captured terminals: %+v want=%v", report.Summary, counts)
	}
}

// Replay exported evidence without contacting GitHub or Docker. SHA checks
// protect the captures; the parser and independent reduction both check them.
func TestSavedPublicRepositoryReportCorpus(t *testing.T) {
	root := os.Getenv("REPOTEST_SAVED_CORPUS")
	if root == "" {
		root = filepath.Join("testdata", "public-corpus")
	}
	manifest, err := os.ReadFile("testdata/public-repositories.json")
	if err != nil {
		t.Fatal(err)
	}
	var repos []struct {
		RepoURL string `json:"repo_url"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(manifest, &repos); err != nil || len(repos) != 10 {
		t.Fatalf("pinned manifest: %v / %d", err, len(repos))
	}
	var environment struct {
		ImageGoVersion string `json:"image_go_version"`
		ManifestSHA    string `json:"source_manifest_sha256"`
	}
	readCorpusJSON(t, filepath.Join(root, "environment.json"), &environment)
	if environment.ManifestSHA != corpusSHA(manifest) || environment.ImageGoVersion == "" {
		t.Fatal("exported corpus has a different pinned manifest or unknown image toolchain")
	}
	for i, repo := range repos {
		t.Run(strings.TrimPrefix(repo.RepoURL, "https://github.com/"), func(t *testing.T) {
			id := fmt.Sprintf("repo-%02d", i+1)
			dir := filepath.Join(root, id)
			var provenance struct {
				RepoURL        string `json:"repo_url"`
				Commit         string `json:"commit"`
				RawSHA         string `json:"raw_sha256"`
				ReportSHA      string `json:"report_sha256"`
				Error          string `json:"error"`
				ExitCode       int    `json:"exit_code"`
				RawTruncated   bool   `json:"raw_truncated"`
				SourceModified bool   `json:"source_modified"`
				FaultInjection bool   `json:"fault_injection"`
			}
			readCorpusJSON(t, filepath.Join(dir, "provenance.json"), &provenance)
			if provenance.RepoURL != repo.RepoURL || provenance.Commit != repo.Commit || provenance.Error != "" || provenance.SourceModified || provenance.FaultInjection {
				t.Fatal("missing/unverified execution or source differs from pinned repository")
			}
			raw, err := os.ReadFile(filepath.Join(dir, "output.raw"))
			if err != nil {
				t.Fatal(err)
			}
			var report domain.Report
			reportBytes := readCorpusJSON(t, filepath.Join(dir, "report.json"), &report)
			if corpusSHA(raw) != provenance.RawSHA || corpusSHA(reportBytes) != provenance.ReportSHA {
				t.Fatal("exported evidence bytes changed")
			}
			verifyPublicCorpusReport(t, raw, report)
			verifyCapturedSourceLocations(t, root, report)
			// Temporary source files are removed by the executor. Source links
			// are checked against independently fetched pinned-file evidence
			// above; parser replay without sources compares all other fields.
			for p := range report.Packages {
				report.Packages[p].FailureLocation = nil
				for c := range report.Packages[p].Tests {
					report.Packages[p].Tests[c].FailureLocation = nil
				}
			}
			parsed := offlineReport(domain.Task{ID: id, RepoURL: repo.RepoURL, Commit: repo.Commit}, environment.ImageGoVersion, Result{ExitCode: provenance.ExitCode, Output: raw, Truncated: provenance.RawTruncated})
			if !reflect.DeepEqual(report, parsed) {
				t.Fatal("current parser differs from exported report fields (excluding source mapping)")
			}
			t.Logf("raw_sha256=%s leaf_summary=%+v", provenance.RawSHA, report.Summary)
		})
	}
}

func verifyCapturedSourceLocations(t *testing.T, root string, report domain.Report) {
	t.Helper()
	var evidence struct {
		Files []struct {
			RepoURL      string `json:"repo_url"`
			Commit       string `json:"commit"`
			Path         string `json:"path"`
			LineCount    int    `json:"line_count"`
			SourceSHA    string `json:"source_sha256"`
			CheckedLines []struct {
				Line int `json:"line"`
			} `json:"checked_lines"`
		} `json:"files"`
	}
	readCorpusJSON(t, filepath.Join(root, "source-verification.json"), &evidence)
	check := func(location *domain.FailureLocation) {
		if location == nil {
			return
		}
		if location.Path == "" || filepath.IsAbs(location.Path) || strings.ContainsAny(location.Path, "\\:%?#") || strings.Contains(location.Path, "..") || location.Line < 1 {
			t.Fatal("unsafe captured source location")
		}
		if location.GitHubURL != report.RepoURL+"/blob/"+report.Commit+"/"+location.Path+"#L"+strconv.Itoa(location.Line) {
			t.Fatal("source link differs from pinned path and line")
		}
		for _, file := range evidence.Files {
			if file.RepoURL == report.RepoURL && file.Commit == report.Commit && file.Path == location.Path {
				if len(file.SourceSHA) != 64 || location.Line > file.LineCount {
					t.Fatal("source file or line not independently verified")
				}
				for _, line := range file.CheckedLines {
					if line.Line == location.Line {
						return
					}
				}
			}
		}
		t.Fatalf("source mapping lacks independent pinned-file evidence: %s:%d", location.Path, location.Line)
	}
	for _, pkg := range report.Packages {
		check(pkg.FailureLocation)
		for _, test := range pkg.Tests {
			check(test.FailureLocation)
		}
	}
}

func readCorpusJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCorpusJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func corpusSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
