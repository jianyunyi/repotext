package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

const publicTaskID = "0123456789abcdef0123456789abcdef"

func publicReportFixture(t *testing.T, status domain.Status, finishedAt time.Time, report *domain.Report) *SQLiteStore {
	t.Helper()
	store := authStore(t)
	createdAt := time.Now().UTC()
	if !finishedAt.IsZero() {
		createdAt = finishedAt.Add(-time.Hour)
	}
	task := domain.Task{ID: publicTaskID, RepoURL: "https://github.com/acme/repo", Ref: "main", Commit: strings.Repeat("a", 40), Status: status, ErrorCode: "private_code", ErrorMessage: "private internal error", CreatedAt: createdAt}
	if !finishedAt.IsZero() {
		task.FinishedAt = &finishedAt
	}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if report != nil {
		body, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutReport(context.Background(), task.ID, body); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func publicRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.20:42310"
	req.Host = "public.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestPublicReportProjectionRedactsPrivateDataAndBoundsDiagnostics(t *testing.T) {
	now := time.Now().UTC()
	longOutput := strings.Repeat("diagnostic-", 700)
	report := domain.Report{TaskID: publicTaskID, RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Output: longOutput, Summary: &domain.ReportSummary{Passed: 1, Failed: 1}, Packages: []domain.Package{
		{ImportPath: "example.com/repo/broken", Status: "failed", Output: longOutput, Tests: []domain.TestCase{{Name: "TestBroken", Status: "failed", Output: longOutput}}},
		{ImportPath: "example.com/repo/ok", Status: "passed", Output: "successful private diagnostic", Tests: []domain.TestCase{{Name: "TestOkay", Status: "passed", Output: "successful private test output"}}},
	}}
	store := publicReportFixture(t, domain.StatusFailed, now, &report)
	response := publicRequest(API{Store: store}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/"+publicTaskID)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("missing public response security headers: %v", response.Header())
	}
	var body struct {
		Task   map[string]any `json:"task"`
		Report domain.Report  `json:"report"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Task["id"] != publicTaskID || body.Task["repo_url"] != "https://github.com/acme/repo" || body.Task["status"] != string(domain.StatusFailed) {
		t.Fatalf("public task summary lost expected fields: %#v", body.Task)
	}
	for _, privateField := range []string{"error_code", "error_message", "attempt"} {
		if _, exists := body.Task[privateField]; exists {
			t.Fatalf("public task exposed %q: %#v", privateField, body.Task)
		}
	}
	if len(body.Report.Output) != 4096 || !body.Report.OutputTruncated {
		t.Fatalf("top-level diagnostic length=%d truncated=%v", len(body.Report.Output), body.Report.OutputTruncated)
	}
	failed := body.Report.Packages[0]
	if len(failed.Output) != 4096 || !failed.OutputTruncated || len(failed.Tests[0].Output) != 4096 || !failed.Tests[0].OutputTruncated {
		t.Fatalf("failed diagnostics were not capped: package=%d/%v test=%d/%v", len(failed.Output), failed.OutputTruncated, len(failed.Tests[0].Output), failed.Tests[0].OutputTruncated)
	}
	passed := body.Report.Packages[1]
	if passed.Output != "" || passed.OutputTruncated || passed.Tests[0].Output != "" || passed.Tests[0].OutputTruncated {
		t.Fatalf("successful diagnostics should not be public: %+v", passed)
	}
	for _, privateText := range []string{"private internal error", "successful private diagnostic", "successful private test output"} {
		if strings.Contains(response.Body.String(), privateText) {
			t.Fatalf("public response leaked %q", privateText)
		}
	}
}

func TestPublicReportAllowsRunningTaskWithoutReport(t *testing.T) {
	store := publicReportFixture(t, domain.StatusRunning, time.Time{}, nil)
	response := publicRequest(API{Store: store}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/"+publicTaskID)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"report":null`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPublicReportRejectsMalformedMissingAndExpiredTasks(t *testing.T) {
	t.Run("malformed id", func(t *testing.T) {
		response := publicRequest(API{Store: authStore(t)}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/not-a-task")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("uppercase id", func(t *testing.T) {
		response := publicRequest(API{Store: authStore(t)}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/0123456789ABCDEF0123456789ABCDEF")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("missing task", func(t *testing.T) {
		response := publicRequest(API{Store: authStore(t)}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/"+publicTaskID)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("expired task", func(t *testing.T) {
		old := time.Now().UTC().Add(-8 * 24 * time.Hour)
		store := publicReportFixture(t, domain.StatusSucceeded, old, &domain.Report{TaskID: publicTaskID})
		response := publicRequest(API{Store: store}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/"+publicTaskID)
		if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "report_expired") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("terminal task without report", func(t *testing.T) {
		store := publicReportFixture(t, domain.StatusFailed, time.Now().UTC(), nil)
		response := publicRequest(API{Store: store}.PublicHandler(), http.MethodGet, "/api/v1/public/reports/"+publicTaskID)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}
