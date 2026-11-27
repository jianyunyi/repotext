package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"repotest/internal/domain"
)

type capturedReportRecord struct {
	ID                        string `json:"id"`
	SourceRoot                string `json:"sourceRoot"`
	RawPath                   string `json:"raw_path"`
	RawSHA256                 string `json:"raw_sha256"`
	ExitCode                  int    `json:"exit_code"`
	Expected                  string `json:"expected"`
	ConsumerExpectedTruncated bool   `json:"consumer_expected_truncated"`
	CoverageReference         struct {
		Available           bool     `json:"available"`
		SemanticPercent     *float64 `json:"report_semantic_percent"`
		NoTestsSemanticNull bool     `json:"no_tests_semantic_null"`
	} `json:"coverage_reference"`
}

// These are actual captured Go tool bytes. Owned fixtures are not evidence of
// executing the ten independent public repositories required at stage exit.
func TestReportCapturedGoOutputGoldens(t *testing.T) {
	root := filepath.Join("testdata", "corpus-capture")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Kind    string                 `json:"kind"`
		Records []capturedReportRecord `json:"records"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "owned_fixture_corpus" || len(manifest.Records) < 20 {
		t.Fatalf("captured-output corpus is missing: %s / %d", manifest.Kind, len(manifest.Records))
	}
	for _, record := range manifest.Records {
		t.Run(record.ID, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, record.RawPath))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if hex.EncodeToString(hash[:]) != record.RawSHA256 {
				t.Fatal("captured raw bytes were changed")
			}
			// The collector preserves complete raw bytes; this reproduces the
			// production consumer's explicit bounded prefix without rewriting it.
			result := Result{ExitCode: record.ExitCode, Output: raw, Truncated: record.ConsumerExpectedTruncated}
			report := offlineReportWithSource(domain.Task{ID: record.ID}, "captured-go-tool", result, filepath.Join(root, record.SourceRoot))
			if report.Summary == nil {
				t.Fatal("missing case summary")
			}
			if record.ConsumerExpectedTruncated {
				if !report.Truncated || !report.Incomplete || report.Summary.Incomplete == 0 {
					t.Fatalf("bounded captured output falsely looked complete: %+v", report)
				}
				return
			}
			switch record.Expected {
			case "success":
				if report.Summary.Passed == 0 || report.Summary.Failed != 0 || report.Incomplete {
					t.Fatalf("successful captured run: %+v", report)
				}
			case "failure":
				if report.Summary.Failed == 0 || report.Summary.PackageFailures == 0 {
					t.Fatalf("failure was not reflected in case/package results: %+v", report)
				}
			case "no_tests":
				if !report.NoTests || report.Summary.Passed != 0 {
					t.Fatalf("no executed tests turned into success: %+v", report)
				}
			case "skipped":
				if report.Summary.Skipped == 0 || report.NoTests {
					t.Fatalf("observed skipped test was lost: %+v", report)
				}
			case "panic":
				if report.Summary.PackageFailures == 0 || report.Summary.Failed+report.Summary.Incomplete == 0 {
					t.Fatalf("panic lost its failure/unfinished case: %+v", report)
				}
			case "build_failure":
				if len(report.Packages) != 1 || report.Summary.PackageFailures != 1 || report.Packages[0].Status != "build_failed" {
					t.Fatalf("test build variant was not attributed once: %+v", report)
				}
				if report.Packages[0].FailureLocation == nil {
					t.Fatal("verified relative compiler location was lost")
				}
			case "go_timeout":
				if report.Summary.PackageFailures == 0 || report.Summary.Passed != 0 {
					t.Fatalf("tool timeout falsely passed: %+v", report)
				}
				// This capture has a coverprofile, but its coverage text is
				// Test-scoped before the timeout panic. It is not a trusted
				// package summary and must not supply report coverage.
				for _, pkg := range report.Packages {
					if pkg.CoveragePct != nil {
						t.Fatalf("test output supplied package coverage: %+v", pkg)
					}
				}
			case "incomplete":
				if !report.Incomplete || report.Summary.Incomplete == 0 || report.Summary.Passed != 0 {
					t.Fatalf("killed tool without terminal falsely passed: %+v", report)
				}
			case "empty_module":
				if report.Summary.Passed != 0 || len(report.Packages) != 0 || report.Output == "" {
					t.Fatalf("empty module lost diagnostic or fabricated results: %+v", report)
				}
			default:
				t.Fatalf("unknown golden intent %q", record.Expected)
			}
			for _, pkg := range report.Packages {
				if record.CoverageReference.NoTestsSemanticNull {
					if pkg.CoveragePct != nil {
						t.Fatalf("no-test coverage must be null: %+v", pkg)
					}
				} else if record.Expected != "go_timeout" && record.CoverageReference.Available && record.CoverageReference.SemanticPercent != nil {
					// Reference comes from statement counts in the independent
					// coverprofile, allowing Go's one-decimal display rounding.
					if pkg.CoveragePct == nil || math.Abs(*pkg.CoveragePct-*record.CoverageReference.SemanticPercent) > 0.051 {
						t.Fatalf("package coverage differs from coverprofile: %+v reference=%v", pkg, *record.CoverageReference.SemanticPercent)
					}
				}
			}
		})
	}
}
