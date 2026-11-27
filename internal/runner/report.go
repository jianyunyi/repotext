package runner

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"repotest/internal/domain"
)

const (
	reportRawLimit           = 2 << 20
	reportCaseOutputLimit    = 16 << 10
	reportPackageOutputLimit = 32 << 10
	reportOutputLimit        = 512 << 10
	reportCaseLimit          = 4096
	reportSerializedLimit    = 8 << 20
	// JSON escaping can expand each byte sixfold. Reserve room for outputs and DTO overhead.
	reportMetadataLimit = 512 << 10
)

var coverageLine = regexp.MustCompile(`^coverage: ([0-9]+(?:\.[0-9]+)?)% of statements(?: in .+)?$`)

type reportEvent struct {
	Action, Package, ImportPath, Test, Output, FailedBuild string
	Elapsed                                                float64
}
type packageEvents struct {
	index         int
	tests         map[string]int
	seen          map[string]bool
	parents       map[string]bool
	noTests       bool
	completedTest bool
}

func offlineReport(task domain.Task, goVersion string, result Result) domain.Report {
	return offlineReportWithSource(task, goVersion, result, "")
}

func offlineReportWithSource(task domain.Task, goVersion string, result Result, sourceRoot string) domain.Report {
	source := indexReportSource(sourceRoot, task)
	report := domain.Report{TaskID: task.ID, GoVersion: goVersion, GOOS: "linux", GOARCH: runtime.GOARCH,
		RepoURL: task.RepoURL, Commit: task.Commit, Truncated: result.Truncated, Incomplete: result.Truncated,
		Packages: []domain.Package{}, Summary: &domain.ReportSummary{}}
	packages := make(map[string]*packageEvents)
	dropped := make(map[string]bool)
	outputBytes, metadataBytes, caseCount := 0, 0, 0
	markLimit := func() { report.Truncated, report.Incomplete = true, true }
	appendOutput := func(dst *string, truncated *bool, output string, limit int) {
		remaining := min(limit-len(*dst), reportOutputLimit-outputBytes)
		if len(output) > remaining {
			*truncated = true
			markLimit()
			output = validPrefix(output, remaining)
		}
		*dst += output
		outputBytes += len(output)
	}
	getPackage := func(name string) *packageEvents {
		if pkg := packages[name]; pkg != nil {
			return pkg
		}
		if len(name) > reportMetadataLimit-metadataBytes {
			markLimit()
			return nil
		}
		metadataBytes += len(name)
		pkg := &packageEvents{index: len(report.Packages), tests: make(map[string]int), seen: make(map[string]bool), parents: make(map[string]bool)}
		packages[name] = pkg
		report.Packages = append(report.Packages, domain.Package{ImportPath: name, Status: "incomplete"})
		return pkg
	}
	data := result.Output
	if len(data) > reportRawLimit {
		data = data[:reportRawLimit]
		markLimit()
	}
	plainPackage := ""
	noPackages := false
	for len(data) > 0 {
		line, rest, _ := bytes.Cut(data, []byte{'\n'})
		data = rest
		var event reportEvent
		if json.Unmarshal(line, &event) != nil || event.Action == "" {
			text := string(line) + "\n"
			if strings.TrimSpace(text) == `go: warning: "./..." matched no packages` {
				noPackages = true
			}
			if strings.HasPrefix(text, "# ") {
				plainPackage = canonicalReportPackage(strings.TrimSpace(strings.TrimPrefix(text, "# ")))
				getPackage(plainPackage)
			}
			if plainPackage != "" && !strings.HasPrefix(text, "go: ") {
				if state := getPackage(plainPackage); state != nil {
					pkg := &report.Packages[state.index]
					appendOutput(&pkg.Output, &pkg.OutputTruncated, text, reportPackageOutputLimit)
					continue
				}
			}
			appendOutput(&report.Output, &report.OutputTruncated, text, reportPackageOutputLimit)
			continue
		}
		plainPackage = ""
		name := event.Package
		if name == "" && strings.HasPrefix(event.Action, "build-") {
			name = event.ImportPath
		}
		if name == "" && event.FailedBuild != "" {
			name = event.FailedBuild
		}
		name = canonicalReportPackage(name)
		if name == "" {
			if event.Output != "" {
				appendOutput(&report.Output, &report.OutputTruncated, event.Output, reportPackageOutputLimit)
			}
			continue
		}
		state := getPackage(name)
		if state == nil {
			if event.Test != "" {
				dropped[name+"\x00"+event.Test] = true
			}
			continue
		}
		pkg := &report.Packages[state.index]
		if event.Test != "" {
			state.seen[event.Test] = true
			if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
				state.completedTest = true
			}
			index, exists := state.tests[event.Test]
			if !exists {
				parent := ""
				if slash := strings.LastIndexByte(event.Test, '/'); slash >= 0 {
					parent = event.Test[:slash]
				}
				if caseCount >= reportCaseLimit || len(event.Test)+len(parent) > reportMetadataLimit-metadataBytes {
					dropped[name+"\x00"+event.Test] = true
					markLimit()
					continue
				}
				metadataBytes += len(event.Test) + len(parent)
				caseCount++
				index = len(pkg.Tests)
				state.tests[event.Test] = index
				pkg.Tests = append(pkg.Tests, domain.TestCase{Name: event.Test, Parent: parent, Status: "incomplete"})
			}
			tc := &pkg.Tests[index]
			tc.Status = terminalStatus(tc.Status, event.Action)
			if elapsed := elapsedMS(event.Elapsed); elapsed > 0 {
				tc.ElapsedMS = elapsed
			}
			if event.Output != "" {
				appendOutput(&tc.Output, &tc.OutputTruncated, event.Output, reportCaseOutputLimit)
			}
			continue
		}
		if event.Output != "" {
			appendOutput(&pkg.Output, &pkg.OutputTruncated, event.Output, reportPackageOutputLimit)
			for _, line := range strings.Split(event.Output, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "?\t"+name+"\t") && strings.Contains(line, "[no test files]") {
					state.noTests = true
				}
				if line == name+"\t\tcoverage: 0.0% of statements" || line == name+"\t\tcoverage: [no statements]" {
					state.noTests = true
				}
				if strings.HasSuffix(line, " [no tests to run]") {
					fields := strings.Fields(line)
					if len(fields) >= 3 && fields[0] == "ok" && fields[1] == name {
						state.noTests = true
					}
				}
				if strings.HasPrefix(line, "FAIL\t"+name) && strings.HasSuffix(line, "[build failed]") {
					pkg.Status = "build_failed"
				}
				if match := coverageLine.FindStringSubmatch(line); match != nil {
					if value, err := strconv.ParseFloat(match[1], 64); err == nil && value >= 0 && value <= 100 {
						pkg.CoveragePct = &value
					}
				}
				if line == "coverage: [no statements]" {
					pkg.CoveragePct = nil
				}
			}
		}
		switch event.Action {
		case "build-fail":
			pkg.Status = "build_failed"
		case "fail":
			if event.FailedBuild != "" || strings.HasPrefix(pkg.Output, "# "+name+"\n") {
				pkg.Status = "build_failed"
			} else if pkg.Status != "build_failed" {
				pkg.Status = "failed"
			}
		case "pass", "skip":
			if pkg.Status != "build_failed" && pkg.Status != "failed" {
				pkg.Status = terminalStatus(pkg.Status, event.Action)
			}
		}
		if elapsed := elapsedMS(event.Elapsed); elapsed > 0 {
			pkg.ElapsedMS = elapsed
		}
	}
	report.Summary.DroppedCases = len(dropped)
	report.NoTests = len(report.Packages) > 0 && !report.Truncated && result.ExitCode == 0
	if len(report.Packages) == 0 {
		report.NoTests = noPackages && !report.Truncated && result.ExitCode == 0
		if !report.NoTests {
			report.Incomplete = true
		}
	}
	for _, state := range packages {
		pkg := &report.Packages[state.index]
		// Include dropped children when determining leaves. Sorted prefix lookups avoid
		// walking every ancestor of attacker-chosen, deeply nested test names.
		names := make([]string, 0, len(state.seen))
		for name := range state.seen {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			prefix := name + "/"
			index := sort.SearchStrings(names, prefix)
			if index < len(names) && strings.HasPrefix(names[index], prefix) {
				state.parents[name] = true
			}
		}
		if pkg.Status == "failed" || pkg.Status == "build_failed" {
			pkg.FailureLocation = source.location(pkg.ImportPath, pkg.Output)
		}
		if state.noTests && len(state.seen) == 0 && (pkg.Status == "skipped" || pkg.Status == "passed") {
			pkg.Status = "no_tests"
			pkg.CoveragePct = nil
		}
		// Package output can originate in init/TestMain before tests run. JSON is
		// not an attestation; without any observed case terminal, omit coverage.
		if !state.completedTest {
			pkg.CoveragePct = nil
		}
		if pkg.Status != "no_tests" {
			report.NoTests = false
		}
		if pkg.Status == "failed" || pkg.Status == "build_failed" {
			report.Summary.PackageFailures++
		}
		if pkg.Status == "incomplete" {
			report.Incomplete = true
		}
		for i := range pkg.Tests {
			tc := &pkg.Tests[i]
			if tc.Status == "failed" || tc.Status == "incomplete" {
				tc.FailureLocation = source.location(pkg.ImportPath, tc.Output)
			}
			if tc.Status == "incomplete" {
				report.Incomplete = true
			}
			if state.parents[tc.Name] {
				continue
			}
			switch tc.Status {
			case "passed":
				report.Summary.Passed++
			case "failed":
				report.Summary.Failed++
			case "skipped":
				report.Summary.Skipped++
			default:
				report.Summary.Incomplete++
			}
		}
		sort.SliceStable(pkg.Tests, func(i, j int) bool {
			return caseRank(pkg.Tests[i], state.parents) < caseRank(pkg.Tests[j], state.parents)
		})
	}
	// Retain a hard guard in addition to byte budgets; do not publish an oversized artifact.
	if encoded, err := json.Marshal(report); err != nil || len(encoded) > reportSerializedLimit {
		markLimit()
		report.Summary = &domain.ReportSummary{DroppedCases: caseCount + len(dropped)}
		report.Packages = []domain.Package{}
		report.Output = ""
		report.OutputTruncated = true
		report.NoTests = false
	}
	return report
}

func terminalStatus(current, action string) string {
	if current == "failed" {
		return current
	}
	switch action {
	case "fail":
		return "failed"
	case "pass":
		return "passed"
	case "skip":
		return "skipped"
	}
	return current
}

// Go identifies rebuilt dependencies and external _test packages by the test
// binary owner in brackets. That owner, not the literal import prefix, owns the
// build diagnostic; FailedBuild references resolve through the same canonical key.
func canonicalReportPackage(name string) string {
	_, variant, found := strings.Cut(name, " [")
	if !found || !strings.HasSuffix(variant, ".test]") {
		return name
	}
	owner := strings.TrimSuffix(variant, ".test]")
	if owner == "" || strings.ContainsAny(owner, " []\\:\x00\r\n\t") {
		return name
	}
	return owner
}

func elapsedMS(elapsed float64) int64 {
	if math.IsNaN(elapsed) || math.IsInf(elapsed, 0) || elapsed <= 0 || elapsed >= 24*60*60 {
		return 0
	}
	return int64(elapsed * 1000)
}
func validPrefix(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for len(text) > 0 {
		r, size := utf8.DecodeLastRuneInString(text)
		if r != utf8.RuneError || size != 1 {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}
func caseRank(tc domain.TestCase, parents map[string]bool) int {
	if tc.Status == "failed" && !parents[tc.Name] {
		return 0
	}
	if tc.Status == "failed" {
		return 1
	}
	if tc.Status == "incomplete" {
		return 2
	}
	return 3
}
