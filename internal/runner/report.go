package runner

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"

	"repotest/internal/domain"
)

func offlineReport(task domain.Task, goVersion string, result Result) domain.Report {
	report := domain.Report{TaskID: task.ID, GoVersion: goVersion, GOOS: "linux", GOARCH: runtime.GOARCH, Truncated: result.Truncated, Packages: []domain.Package{}}
	indices := make(map[string]int)
	for data := result.Output; len(data) > 0; {
		var line []byte
		line, data, _ = bytes.Cut(data, []byte{'\n'})
		var event struct {
			Action, Package, ImportPath, Test, Output, FailedBuild string
			Elapsed                                                float64
		}
		if json.Unmarshal(line, &event) != nil || event.Test != "" {
			continue
		}
		name := event.Package
		if event.Action == "build-fail" && name == "" {
			name = event.ImportPath
		}
		if name == "" {
			continue
		}
		index, exists := indices[name]
		if !exists {
			index = len(report.Packages)
			indices[name] = index
			report.Packages = append(report.Packages, domain.Package{ImportPath: name, Status: "incomplete"})
		}
		pkg := &report.Packages[index]
		switch event.Action {
		case "pass":
			pkg.Status = "passed"
		case "fail":
			if event.FailedBuild != "" || pkg.Status == "build_failed" {
				pkg.Status = "build_failed"
			} else {
				pkg.Status = "failed"
			}
		case "build-fail":
			pkg.Status = "build_failed"
		case "skip":
			pkg.Status = "skipped"
			if pkg.Output == "no test files" {
				pkg.Status = "no_tests"
			}
		case "output":
			if strings.TrimSpace(event.Output) == "?\t"+name+"\t[no test files]" {
				pkg.Output = "no test files"
			}
		}
		if event.Elapsed > 0 && event.Elapsed < 24*60*60 {
			pkg.ElapsedMS = int64(event.Elapsed * 1000)
		}
	}
	report.NoTests = len(report.Packages) > 0 && !result.Truncated && result.ExitCode == 0
	for _, pkg := range report.Packages {
		if pkg.Status != "no_tests" {
			report.NoTests = false
			break
		}
	}
	return report
}
