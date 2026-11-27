package control

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

func TestPublicHandlerServesAnonymousReadOnlyReportPage(t *testing.T) {
	now := time.Now().UTC()
	report := domain.Report{TaskID: publicTaskID, RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Summary: &domain.ReportSummary{Passed: 12, Failed: 2}}
	store := publicReportFixture(t, domain.StatusFailed, now, &report)
	handler := API{Store: store}.PublicHandler()
	response := publicRequest(handler, http.MethodGet, "/r/"+publicTaskID)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `property="og:title"`) || !strings.Contains(response.Body.String(), `property="og:description"`) || !strings.Contains(response.Body.String(), "acme/repo") {
		t.Fatalf("Open Graph metadata missing: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "__TASK_ID__") || !strings.Contains(response.Body.String(), publicTaskID) {
		t.Fatalf("task ID was not rendered safely into the public page: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "__CSRF_TOKEN__") || strings.Contains(response.Body.String(), "repotest-csrf") || strings.Contains(response.Body.String(), "/api/v1/tasks/") || strings.Contains(response.Body.String(), "/api/v1/sessions") {
		t.Fatalf("public page contains local-only or mutation UI: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "<script>") || strings.Contains(response.Body.String(), "<style>") {
		t.Fatalf("public page contains inline code incompatible with its CSP: %s", response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'none'") || !strings.Contains(response.Header().Get("Content-Security-Policy"), "connect-src 'self'") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("inadequate public page headers: %v", response.Header())
	}
}

func TestPublicHandlerRouteTableExcludesMutationsLogsAndSessions(t *testing.T) {
	store := authStore(t)
	task := domain.Task{ID: publicTaskID, Status: domain.StatusSucceeded, CreatedAt: time.Now().UTC()}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReport(context.Background(), publicTaskID, []byte(`{"task_id":"`+publicTaskID+`","packages":[]}`)); err != nil {
		t.Fatal(err)
	}
	handler := API{Store: store}.PublicHandler()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/previews"},
		{http.MethodPost, "/api/v1/tasks"},
		{http.MethodPost, "/api/v1/tasks/" + publicTaskID + "/cancel"},
		{http.MethodDelete, "/api/v1/tasks/" + publicTaskID},
		{http.MethodGet, "/api/v1/tasks/" + publicTaskID + "/log"},
		{http.MethodGet, "/api/v1/sessions"},
		{http.MethodDelete, "/api/v1/sessions/device"},
		{http.MethodPost, "/r/" + publicTaskID},
		{http.MethodGet, "/private"},
	} {
		response := publicRequest(handler, tc.method, tc.path)
		if response.Code >= http.StatusOK && response.Code < http.StatusMultipleChoices {
			t.Errorf("unexpected success for %s %s: %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}

func TestPublicHandlerServesOnlyApprovedStaticAssets(t *testing.T) {
	handler := API{}.PublicHandler()
	for _, asset := range []struct{ path, contentType, marker string }{
		{"/assets/report.js", "text/javascript; charset=utf-8", "RepoTestReport"},
		{"/assets/report.css", "text/css; charset=utf-8", ".report"},
		{"/assets/public.js", "text/javascript; charset=utf-8", "RepoTestReport.render"},
		{"/assets/public.css", "text/css; charset=utf-8", ".public-report"},
	} {
		response := publicRequest(handler, http.MethodGet, asset.path)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != asset.contentType || !strings.Contains(response.Body.String(), asset.marker) {
			t.Errorf("asset %s status=%d type=%q body=%q", asset.path, response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("asset %s missing security headers: %v", asset.path, response.Header())
		}
	}
	for _, path := range []string{"/assets/missing.js", "/assets/web/index.html"} {
		response := publicRequest(handler, http.MethodGet, path)
		if response.Code != http.StatusNotFound {
			t.Errorf("unapproved asset %s returned %d", path, response.Code)
		}
	}
}

func TestPublicPageAssetsUseOnlyTextDOMAPIs(t *testing.T) {
	handler := API{}.PublicHandler()
	response := publicRequest(handler, http.MethodGet, "/assets/public.js")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"innerHTML", "insertAdjacentHTML", "__CSRF_TOKEN__", "/api/v1/tasks/", "/api/v1/sessions"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("public bootstrap contains %q", forbidden)
		}
	}
	if !strings.Contains(response.Body.String(), "textContent") || !strings.Contains(response.Body.String(), "RepoTestReport.render") {
		t.Fatalf("public bootstrap does not use the shared safe renderer and text nodes: %s", response.Body.String())
	}
}

func TestPublicPageExplainsExpiredAndUnavailableReports(t *testing.T) {
	now := time.Now().UTC()
	store := publicReportFixture(t, domain.StatusSucceeded, now.Add(-8*24*time.Hour), &domain.Report{TaskID: publicTaskID})
	handler := API{Store: store}.PublicHandler()

	expired := publicRequest(handler, http.MethodGet, "/r/"+publicTaskID)
	if expired.Code != http.StatusGone || !strings.Contains(expired.Header().Get("Content-Type"), "text/html") || !strings.Contains(expired.Body.String(), "报告已过期") {
		t.Fatalf("expired report should render an explanatory HTML page: status=%d headers=%v body=%s", expired.Code, expired.Header(), expired.Body.String())
	}
	if strings.Contains(expired.Body.String(), "public.js") || strings.Contains(expired.Body.String(), "提交新任务") {
		t.Fatalf("expired page must not load report bootstrap or claim public submission is available: %s", expired.Body.String())
	}

	unavailable := publicRequest(API{Store: authStore(t)}.PublicHandler(), http.MethodGet, "/r/"+publicTaskID)
	if unavailable.Code != http.StatusNotFound || !strings.Contains(unavailable.Header().Get("Content-Type"), "text/html") || !strings.Contains(unavailable.Body.String(), "报告不存在、已过期或已清除") {
		t.Fatalf("unavailable report should render an explanatory HTML page: status=%d headers=%v body=%s", unavailable.Code, unavailable.Header(), unavailable.Body.String())
	}
	if strings.Contains(unavailable.Body.String(), "public.js") || strings.Contains(unavailable.Body.String(), "提交新任务") {
		t.Fatalf("unavailable page must not load report bootstrap or expose a submission action: %s", unavailable.Body.String())
	}
}

func TestPublicPageExplainsStorageUnavailableWithoutDetails(t *testing.T) {
	response := publicRequest(API{}.PublicHandler(), http.MethodGet, "/r/"+publicTaskID)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Header().Get("Content-Type"), "text/html") || !strings.Contains(response.Body.String(), "报告暂时无法读取，请稍后重试") {
		t.Fatalf("storage failure should render a generic HTML 503 page: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if strings.Contains(response.Body.String(), "public.js") || strings.Contains(response.Body.String(), "public report storage unavailable") {
		t.Fatalf("storage error page must not load report bootstrap or expose internal details: %s", response.Body.String())
	}
}
