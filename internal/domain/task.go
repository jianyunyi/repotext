package domain

import "time"

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimeout   Status = "timeout"
	StatusCancelled Status = "cancelled"
	StatusError     Status = "error"
)

type Phase string

const (
	PhaseClone Phase = "clone"
	PhaseDeps  Phase = "deps"
	PhaseBuild Phase = "build"
	PhaseTest  Phase = "test"
)

type Task struct {
	ID           string     `json:"id"`
	RepoURL      string     `json:"repo_url"`
	Ref          string     `json:"ref,omitempty"`
	Commit       string     `json:"commit,omitempty"`
	Status       Status     `json:"status"`
	Phase        Phase      `json:"phase,omitempty"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Attempt      int        `json:"attempt"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}
type Package struct {
	ImportPath      string           `json:"import_path"`
	Status          string           `json:"status"`
	ElapsedMS       int64            `json:"elapsed_ms,omitempty"`
	CoveragePct     *float64         `json:"coverage_pct,omitempty"`
	Output          string           `json:"output,omitempty"`
	OutputTruncated bool             `json:"output_truncated,omitempty"`
	Tests           []TestCase       `json:"tests,omitempty"`
	FailureLocation *FailureLocation `json:"failure_location,omitempty"`
}
type FailureLocation struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	GitHubURL string `json:"github_url,omitempty"`
}
type TestCase struct {
	Name            string           `json:"name"`
	Parent          string           `json:"parent,omitempty"`
	Status          string           `json:"status"`
	ElapsedMS       int64            `json:"elapsed_ms,omitempty"`
	Output          string           `json:"output,omitempty"`
	OutputTruncated bool             `json:"output_truncated,omitempty"`
	FailureLocation *FailureLocation `json:"failure_location,omitempty"`
}
type ReportSummary struct {
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Skipped         int `json:"skipped"`
	Incomplete      int `json:"incomplete"`
	PackageFailures int `json:"package_failures"`
	DroppedCases    int `json:"dropped_cases,omitempty"`
}
type Report struct {
	TaskID          string         `json:"task_id"`
	GoVersion       string         `json:"go_version,omitempty"`
	GOOS            string         `json:"goos"`
	GOARCH          string         `json:"goarch"`
	NoTests         bool           `json:"no_tests"`
	Truncated       bool           `json:"truncated"`
	Packages        []Package      `json:"packages"`
	RepoURL         string         `json:"repo_url,omitempty"`
	Commit          string         `json:"commit,omitempty"`
	Summary         *ReportSummary `json:"summary,omitempty"`
	Incomplete      bool           `json:"incomplete,omitempty"`
	Output          string         `json:"output,omitempty"`
	OutputTruncated bool           `json:"output_truncated,omitempty"`
}
