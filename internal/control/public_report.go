package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"repotest/internal/domain"
)

const publicReportTTL = 7 * 24 * time.Hour
const maxPublicDiagnosticBytes = 4096

var publicTaskIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var (
	errPublicReportNotFound = errors.New("public report not found")
	errPublicReportExpired  = errors.New("public report expired")
	errPublicStorage        = errors.New("public report storage unavailable")
)

type publicReportStore interface {
	Get(context.Context, string) (domain.Task, error)
	GetReport(context.Context, string) ([]byte, error)
}

type publicTask struct {
	ID         string        `json:"id"`
	RepoURL    string        `json:"repo_url"`
	Ref        string        `json:"ref,omitempty"`
	Commit     string        `json:"commit,omitempty"`
	Status     domain.Status `json:"status"`
	Phase      domain.Phase  `json:"phase,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	StartedAt  *time.Time    `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
}

type publicReportResponse struct {
	Task   publicTask     `json:"task"`
	Report *domain.Report `json:"report"`
}

func (a API) loadPublicReport(ctx context.Context, id string) (publicReportResponse, error) {
	if !publicTaskIDPattern.MatchString(id) {
		return publicReportResponse{}, errPublicReportNotFound
	}
	store, ok := a.Store.(publicReportStore)
	if !ok {
		return publicReportResponse{}, errPublicStorage
	}
	task, err := store.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return publicReportResponse{}, errPublicReportNotFound
	}
	if err != nil {
		return publicReportResponse{}, errPublicStorage
	}
	if task.FinishedAt != nil && !time.Now().UTC().Before(task.FinishedAt.Add(publicReportTTL)) {
		return publicReportResponse{}, errPublicReportExpired
	}
	response := publicReportResponse{Task: publicTask{
		ID:         task.ID,
		RepoURL:    task.RepoURL,
		Ref:        task.Ref,
		Commit:     task.Commit,
		Status:     task.Status,
		Phase:      task.Phase,
		CreatedAt:  task.CreatedAt,
		StartedAt:  task.StartedAt,
		FinishedAt: task.FinishedAt,
	}}
	stored, err := store.GetReport(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		if task.Status == domain.StatusQueued || task.Status == domain.StatusRunning || task.Status == domain.StatusCancelled {
			return response, nil
		}
		return publicReportResponse{}, errPublicReportNotFound
	}
	if err != nil {
		return publicReportResponse{}, errPublicStorage
	}
	var report domain.Report
	if err := json.Unmarshal(stored, &report); err != nil {
		return publicReportResponse{}, errPublicStorage
	}
	response.Report = sanitizePublicReport(report, task)
	return response, nil
}

func sanitizePublicReport(report domain.Report, task domain.Task) *domain.Report {
	public := report
	public.TaskID = task.ID
	public.RepoURL = task.RepoURL
	public.Commit = task.Commit
	public.Output, public.OutputTruncated = publicDiagnostic(report.Output, report.OutputTruncated, task.Status == domain.StatusFailed || task.Status == domain.StatusTimeout || task.Status == domain.StatusError || report.Incomplete)
	public.Packages = append([]domain.Package(nil), report.Packages...)
	for packageIndex := range public.Packages {
		pkg := &public.Packages[packageIndex]
		pkg.Output, pkg.OutputTruncated = publicDiagnostic(pkg.Output, pkg.OutputTruncated, pkg.Status == "failed" || pkg.Status == "build_failed" || pkg.Status == "incomplete")
		pkg.Tests = append([]domain.TestCase(nil), pkg.Tests...)
		for testIndex := range pkg.Tests {
			testCase := &pkg.Tests[testIndex]
			testCase.Output, testCase.OutputTruncated = publicDiagnostic(testCase.Output, testCase.OutputTruncated, testCase.Status == "failed" || testCase.Status == "incomplete")
			if testCase.Status != "failed" && testCase.Status != "incomplete" {
				testCase.FailureLocation = nil
			}
		}
		if pkg.Status != "failed" && pkg.Status != "build_failed" && pkg.Status != "incomplete" {
			pkg.FailureLocation = nil
		}
	}
	return &public
}

func publicDiagnostic(value string, truncated, allowed bool) (string, bool) {
	if !allowed {
		return "", false
	}
	if len(value) <= maxPublicDiagnosticBytes {
		return value, truncated
	}
	limit := maxPublicDiagnosticBytes
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return strings.Clone(value[:limit]), true
}
