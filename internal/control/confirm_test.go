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

func TestPreviewConfirmationCreatesAndRetriesOneTask(t *testing.T) {
	store := authStore(t)
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/repos/acme/widget") {
			return jsonResponse(http.StatusOK, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":10}`), nil
		}
		return jsonResponse(http.StatusOK, `{"object":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","type":"commit"}}`), nil
	})}
	api := API{Store: store, GitHubClient: client, CSRFToken: testCSRFToken, RunnerEnabled: true, GoVersion: "go1.27.1", RunnerVersion: "sha256:" + strings.Repeat("b", 64), RunnerConfig: []byte("bounded-offline-v1")}
	handler := api.Handler()
	previewRequest := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"main"}`)))
	previewResponse := httptest.NewRecorder()
	handler.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview Preview
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil || preview.PreviewID == "" || preview.CommitSHA != strings.Repeat("a", 40) {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	confirmBody, _ := json.Marshal(confirmRequest{PreviewID: preview.PreviewID})
	confirm := func() *httptest.ResponseRecorder {
		req := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(string(confirmBody))))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	first := confirm()
	if first.Code != http.StatusAccepted {
		t.Fatalf("confirm status=%d body=%s", first.Code, first.Body.String())
	}
	var firstBody struct {
		Task   domain.Task `json:"task"`
		Reused bool        `json:"reused"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil || firstBody.Task.Status != domain.StatusQueued || firstBody.Reused {
		t.Fatalf("confirm=%+v err=%v", firstBody, err)
	}
	second := confirm()
	var secondBody struct {
		Task   domain.Task `json:"task"`
		Reused bool        `json:"reused"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil || second.Code != http.StatusOK || !secondBody.Reused || secondBody.Task.ID != firstBody.Task.ID {
		t.Fatalf("retry status=%d task=%+v reused=%v err=%v", second.Code, secondBody.Task, secondBody.Reused, err)
	}
	var tasks, outbox int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM outbox").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || outbox != 1 {
		t.Fatalf("tasks=%d outbox=%d", tasks, outbox)
	}
}

func TestConfirmationDisabledAndLANMutationStayClosed(t *testing.T) {
	store := authStore(t)
	req := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"preview_id":"AA"}`)))
	rec := httptest.NewRecorder()
	(API{Store: store, CSRFToken: testCSRFToken}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "runner_disabled") {
		t.Fatalf("disabled confirmation status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.Get(context.Background(), "none"); err == nil {
		t.Fatal("disabled confirmation created task")
	}
	pairing, _, err := NewPairing(store, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lan := NewLANHandler(API{Store: store}, pairing, []string{"127.0.0.1"})
	lanRequest := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"preview_id":"AA"}`))
	lanRequest.Host = "127.0.0.1"
	lanRequest.RemoteAddr = "127.0.0.1:12345"
	lanResponse := httptest.NewRecorder()
	lan.ServeHTTP(lanResponse, lanRequest)
	if lanResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("LAN confirmation status=%d", lanResponse.Code)
	}
}

func TestReportEndpointServesPersistedJSON(t *testing.T) {
	store := authStore(t)
	task := domain.Task{ID: "report-task", RepoURL: "https://github.com/acme/repo", Commit: strings.Repeat("a", 40), Status: domain.StatusSucceeded, CreatedAt: time.Now()}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReport(context.Background(), task.ID, []byte(`{"task_id":"report-task","packages":[{"import_path":"example.test/pkg","status":"passed"}]}`)); err != nil {
		t.Fatal(err)
	}
	req := loopbackRequest(httptest.NewRequest(http.MethodGet, "/api/v1/tasks/report-task/report", nil))
	rec := httptest.NewRecorder()
	(API{Store: store, CSRFToken: testCSRFToken}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"passed"`) {
		t.Fatalf("report status=%d body=%s", rec.Code, rec.Body.String())
	}
}
