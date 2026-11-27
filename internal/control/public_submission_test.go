package control

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"repotest/internal/domain"
)

func publicSubmissionAPI(t *testing.T, captcha CaptchaVerifier) (API, *SQLiteStore, *int) {
	t.Helper()
	store := authStore(t)
	calls := new(int)
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		(*calls)++
		if strings.HasSuffix(r.URL.Path, "/repos/acme/widget") {
			return jsonResponse(http.StatusOK, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":10}`), nil
		}
		return jsonResponse(http.StatusOK, `{"object":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","type":"commit"}}`), nil
	})}
	return API{Store: store, GitHubClient: client, RunnerEnabled: true, GoVersion: "go1.23", RunnerVersion: "sha256:" + strings.Repeat("b", 64), RunnerConfig: []byte("bounded-offline-v1"), Captcha: captcha, PublicOrigins: []string{"https://public.example"}}, store, calls
}

func TestPublicPreviewFailsClosedWhenCaptchaIsUnavailable(t *testing.T) {
	api, _, calls := publicSubmissionAPI(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","captcha_token":"ok"}`))
	req.RemoteAddr = "203.0.113.20:42310"
	res := httptest.NewRecorder()
	api.PublicHandler().ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), "captcha_unavailable") || *calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", res.Code, *calls, res.Body.String())
	}
}

func TestPublicPreviewRejectsEmptyAndInvalidCaptchaBeforeGitHubRead(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		captcha               string
		reject                bool
		wantStatus            int
		wantVerifierCallCount int
	}{
		{name: "empty token", captcha: "", wantStatus: http.StatusForbidden},
		{name: "rejected token", captcha: "invalid", reject: true, wantStatus: http.StatusForbidden, wantVerifierCallCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verifierCalls := 0
			verifier := CaptchaVerifierFunc(func(context.Context, string, string) error {
				verifierCalls++
				if tc.reject {
					return errors.New("rejected")
				}
				return nil
			})
			api, store, calls := publicSubmissionAPI(t, verifier)
			body := `{"repo_url":"https://github.com/acme/widget","captcha_token":"` + tc.captcha + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/public/previews", strings.NewReader(body))
			res := httptest.NewRecorder()
			api.PublicHandler().ServeHTTP(res, req)
			if res.Code != tc.wantStatus || !strings.Contains(res.Body.String(), "captcha_failed") || *calls != 0 || verifierCalls != tc.wantVerifierCallCount {
				t.Fatalf("status=%d verifier calls=%d github calls=%d body=%s", res.Code, verifierCalls, *calls, res.Body.String())
			}
			var previews, tasks int
			if err := store.db.QueryRow("SELECT COUNT(*) FROM previews").Scan(&previews); err != nil {
				t.Fatal(err)
			}
			if err := store.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks); err != nil {
				t.Fatal(err)
			}
			if previews != 0 || tasks != 0 {
				t.Fatalf("created previews=%d tasks=%d", previews, tasks)
			}
		})
	}
}

func TestPublicSubmissionReturnsOneTimeControlTokenAndAuthorizesRemoteCancel(t *testing.T) {
	api, store, _ := publicSubmissionAPI(t, CaptchaVerifierFunc(func(context.Context, string, string) error { return nil }))
	handler := api.PublicHandler()
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"main","captcha_token":"ok"}`))
	previewReq.RemoteAddr = "203.0.113.20:42310"
	previewReq.Header.Set("Origin", "https://public.example")
	previewRes := httptest.NewRecorder()
	handler.ServeHTTP(previewRes, previewReq)
	if previewRes.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRes.Code, previewRes.Body.String())
	}
	var preview Preview
	if err := json.Unmarshal(previewRes.Body.Bytes(), &preview); err != nil || preview.PreviewID == "" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	confirmRes := httptest.NewRecorder()
	confirmReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/tasks", strings.NewReader(`{"preview_id":"`+preview.PreviewID+`"}`))
	confirmReq.RemoteAddr = "203.0.113.20:42310"
	confirmReq.Header.Set("Origin", "https://public.example")
	handler.ServeHTTP(confirmRes, confirmReq)
	if confirmRes.Code != http.StatusAccepted {
		t.Fatalf("confirm status=%d body=%s", confirmRes.Code, confirmRes.Body.String())
	}
	var created struct {
		Task struct {
			ID                string `json:"id"`
			Status            string `json:"status"`
			ExecutionIdentity any    `json:"execution_identity"`
		} `json:"task"`
		ControlToken string `json:"control_token"`
	}
	if err := json.Unmarshal(confirmRes.Body.Bytes(), &created); err != nil || created.Task.ID == "" || created.ControlToken == "" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	if created.Task.ExecutionIdentity != nil {
		t.Fatal("public response exposed the internal execution identity")
	}
	persisted, err := store.Get(context.Background(), created.Task.ID)
	if err != nil || persisted.ExecutionIdentity == nil || persisted.ExecutionIdentity.RepoURL != "https://github.com/acme/widget" || persisted.ExecutionIdentity.CommitSHA != strings.Repeat("a", 40) || persisted.ExecutionIdentity.GoVersion != api.GoVersion || persisted.ExecutionIdentity.RunnerVersion != api.RunnerVersion || persisted.ExecutionIdentity.ConfigDigest != ConfigurationDigest(api.RunnerConfig) {
		t.Fatalf("persisted execution identity=%+v err=%v", persisted.ExecutionIdentity, err)
	}
	var hash []byte
	if err := store.db.QueryRow("SELECT token_hash FROM task_tokens WHERE task_id=?", created.Task.ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(mustDecodeToken(t, created.ControlToken))
	if len(hash) != len(wantHash) || string(hash) != string(wantHash[:]) || strings.Contains(string(hash), created.ControlToken) {
		t.Fatalf("token hash not persisted safely: %x", hash)
	}
	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/tasks", strings.NewReader(`{"preview_id":"`+preview.PreviewID+`"}`))
	retryReq.Header.Set("Origin", "https://public.example")
	retryRes := httptest.NewRecorder()
	handler.ServeHTTP(retryRes, retryReq)
	if retryRes.Code != http.StatusOK || strings.Contains(retryRes.Body.String(), "control_token") {
		t.Fatalf("repeat confirmation status=%d body=%s", retryRes.Code, retryRes.Body.String())
	}
	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/public/tasks/"+created.Task.ID+"/cancel", nil)
	wrong.RemoteAddr = "198.51.100.9:4000"
	wrong.Header.Set("Authorization", "Bearer invalid")
	wrongRes := httptest.NewRecorder()
	handler.ServeHTTP(wrongRes, wrong)
	if wrongRes.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status=%d body=%s", wrongRes.Code, wrongRes.Body.String())
	}
	cancel := httptest.NewRequest(http.MethodPost, "/api/v1/public/tasks/"+created.Task.ID+"/cancel", nil)
	cancel.RemoteAddr = "198.51.100.9:4000"
	cancel.Header.Set("Authorization", "Bearer "+created.ControlToken)
	cancelRes := httptest.NewRecorder()
	handler.ServeHTTP(cancelRes, cancel)
	if cancelRes.Code != http.StatusNoContent {
		t.Fatalf("remote cancel status=%d body=%s", cancelRes.Code, cancelRes.Body.String())
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/public/tasks/"+created.Task.ID, nil)
	deleteReq.RemoteAddr = "198.51.100.9:4000"
	deleteReq.Header.Set("Authorization", "Bearer "+created.ControlToken)
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("remote delete status=%d body=%s", deleteRes.Code, deleteRes.Body.String())
	}
	if _, err := store.Get(context.Background(), created.Task.ID); err == nil {
		t.Fatal("task remains after authorized delete")
	}
}

type taskProcessesFunc func(context.Context, string) error

func (f taskProcessesFunc) CancelAndWait(ctx context.Context, taskID string) error {
	return f(ctx, taskID)
}

func TestPublicDeletePreservesTaskWhenCleanupIsPending(t *testing.T) {
	store := authStore(t)
	taskID := "0123456789abcdef0123456789abcdef"
	task := domain.Task{ID: taskID, RepoURL: "https://github.com/acme/widget", Commit: strings.Repeat("a", 40), Status: domain.StatusRunning}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	controlToken := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("t", 32)))
	controlHash := sha256.Sum256(mustDecodeToken(t, controlToken))
	if err := store.StoreTaskToken(context.Background(), taskID, controlHash[:]); err != nil {
		t.Fatal(err)
	}
	api := API{Store: store, Processes: taskProcessesFunc(func(context.Context, string) error { return ErrCleanupPending })}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/public/tasks/" + taskID + "/cancel"},
		{http.MethodDelete, "/api/v1/public/tasks/" + taskID},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "198.51.100.9:4000"
		req.Header.Set("Authorization", "Bearer "+controlToken)
		res := httptest.NewRecorder()
		api.PublicHandler().ServeHTTP(res, req)
		if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "cleanup_pending") {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, res.Code, res.Body.String())
		}
		got, err := store.Get(context.Background(), taskID)
		if err != nil || got.Status != domain.StatusRunning {
			t.Fatalf("task after %s cleanup failure=%+v err=%v", tc.method, got, err)
		}
	}
}

func TestPublicMutationRejectsDisallowedOrigin(t *testing.T) {
	store := authStore(t)
	taskID := "0123456789abcdef0123456789abcdef"
	if err := store.Create(context.Background(), domain.Task{ID: taskID, RepoURL: "https://github.com/acme/widget", Commit: strings.Repeat("a", 40), Status: domain.StatusQueued}); err != nil {
		t.Fatal(err)
	}
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	hash := sha256.Sum256(mustDecodeToken(t, token))
	if err := store.StoreTaskToken(context.Background(), taskID, hash[:]); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/tasks/"+taskID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://evil.example")
	res := httptest.NewRecorder()
	(API{Store: store, PublicOrigins: []string{"https://public.example"}}).PublicHandler().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("origin status=%d body=%s", res.Code, res.Body.String())
	}
}

func mustDecodeToken(t *testing.T, token string) []byte {
	t.Helper()
	value, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
