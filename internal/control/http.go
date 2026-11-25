package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"

	"repotest/internal/domain"
	"repotest/internal/runner"
)

type API struct {
	Store         Store
	GitHubClient  *http.Client
	CSRFToken     string
	Processes     TaskProcesses
	RunnerEnabled bool
	GoVersion     string
	RunnerVersion string
	RunnerConfig  []byte
}
type Preview struct {
	RepoURL       string     `json:"repo_url"`
	FullName      string     `json:"full_name"`
	Private       bool       `json:"private"`
	Archived      bool       `json:"archived"`
	DefaultBranch string     `json:"default_branch"`
	Ref           string     `json:"ref"`
	CommitSHA     string     `json:"commit_sha"`
	SizeKB        int64      `json:"size_kb"`
	GOOS          string     `json:"goos"`
	GOARCH        string     `json:"goarch"`
	PreviewID     string     `json:"preview_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}
type previewRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref,omitempty"`
}

var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func (a API) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
		writeError(w, 400, "invalid_request")
		return
	}
	repo, err := runner.ValidateRepoURL(req.RepoURL)
	if err != nil {
		writeError(w, 400, "invalid_url")
		return
	}
	owner, name, ok := repoParts(repo)
	if !ok {
		writeError(w, 400, "invalid_url")
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		ref = "HEAD"
	}
	if !validRef(ref) {
		writeError(w, 400, "invalid_ref")
		return
	}
	client := a.GitHubClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	var meta struct {
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		Archived      bool   `json:"archived"`
		DefaultBranch string `json:"default_branch"`
		Size          int64  `json:"size"`
	}
	if err := githubGet(r.Context(), client, "https://api.github.com/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), &meta); err != nil {
		writeGitHubError(w, err)
		return
	}
	if meta.Private {
		writeError(w, 403, "repo_private")
		return
	}
	if meta.Archived {
		writeError(w, 400, "repo_archived")
		return
	}
	if meta.Size > 100*1024 {
		writeError(w, 413, "repo_too_large")
		return
	}
	defaultRef := ref == "HEAD"
	if defaultRef {
		ref = meta.DefaultBranch
	}
	var sha string
	if fullSHA.MatchString(ref) {
		sha = strings.ToLower(ref)
		var commit struct {
			SHA string `json:"sha"`
		}
		u := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/commits/" + sha
		if err := githubGet(r.Context(), client, u, &commit); err != nil {
			var ge githubStatusError
			if errors.As(err, &ge) && (ge.status == 404 || ge.status == 422) {
				writeError(w, 404, "repo_or_ref_not_found")
			} else {
				writeGitHubError(w, err)
			}
			return
		}
		if !strings.EqualFold(commit.SHA, sha) {
			writeError(w, 404, "repo_or_ref_not_found")
			return
		}
	} else {
		kind, value := "heads", ref
		if strings.HasPrefix(ref, "refs/tags/") {
			kind, value = "tags", strings.TrimPrefix(ref, "refs/tags/")
		} else if strings.HasPrefix(ref, "refs/heads/") {
			kind, value = "heads", strings.TrimPrefix(ref, "refs/heads/")
		} else if !defaultRef && ref != meta.DefaultBranch {
			kind = "tags"
		}
		sha, err = resolveRef(r.Context(), client, owner, name, kind, value)
		if err != nil && kind == "tags" && !strings.HasPrefix(ref, "refs/") && !defaultRef && ref != meta.DefaultBranch {
			var ge githubStatusError
			if errors.As(err, &ge) && ge.status == 404 {
				sha, err = resolveRef(r.Context(), client, owner, name, "heads", value)
			}
		}
		if err != nil {
			writeGitHubError(w, err)
			return
		}
	}
	preview := Preview{RepoURL: repo, FullName: meta.FullName, Private: meta.Private, Archived: meta.Archived, DefaultBranch: meta.DefaultBranch, Ref: ref, CommitSHA: sha, SizeKB: meta.Size, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if a.RunnerEnabled {
		store, ok := a.Store.(previewTokenStore)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
			return
		}
		token := make([]byte, 32)
		if _, err = rand.Read(token); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		now := time.Now().UTC()
		expires := now.Add(10 * time.Minute)
		hash := sha256.Sum256(token)
		if err = store.StorePreview(r.Context(), hash[:], StoredPreview{RepoURL: repo, Ref: ref, CommitSHA: sha, ExpiresAt: expires}); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		preview.PreviewID = base64.RawURLEncoding.EncodeToString(token)
		preview.ExpiresAt = &expires
	}
	writeJSON(w, 200, preview)
}

type previewTokenStore interface {
	StorePreview(context.Context, []byte, StoredPreview) error
}

type previewConfirmationStore interface {
	ConfirmPreview(context.Context, []byte, domain.Task, ExecutionIdentity, time.Time, time.Duration) (domain.Task, bool, error)
}

type confirmRequest struct {
	PreviewID string `json:"preview_id"`
}

func (a API) confirm(w http.ResponseWriter, r *http.Request) {
	if !a.RunnerEnabled {
		writeError(w, http.StatusServiceUnavailable, "runner_disabled")
		return
	}
	if strings.TrimSpace(a.GoVersion) == "" || strings.TrimSpace(a.RunnerVersion) == "" {
		writeError(w, http.StatusServiceUnavailable, "runner_not_configured")
		return
	}
	var req confirmRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	token, err := base64.RawURLEncoding.DecodeString(req.PreviewID)
	if err != nil || len(token) != 32 {
		writeError(w, http.StatusBadRequest, "invalid_preview")
		return
	}
	store, ok := a.Store.(previewConfirmationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	var idBytes [16]byte
	if _, err = rand.Read(idBytes[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	now := time.Now().UTC()
	task := domain.Task{ID: hex.EncodeToString(idBytes[:]), Status: domain.StatusQueued, CreatedAt: now}
	identity := ExecutionIdentity{goVersion: a.GoVersion, runnerVersion: a.RunnerVersion, config: a.RunnerConfig}
	hash := sha256.Sum256(token)
	created, reused, err := store.ConfirmPreview(r.Context(), hash[:], task, identity, now, 10*time.Minute)
	if err != nil {
		writeTaskMutationError(w, err)
		return
	}
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"task": created, "reused": reused})
}

func (a API) report(w http.ResponseWriter, r *http.Request) {
	store, ok := a.Store.(interface {
		GetReport(context.Context, string) ([]byte, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	body, err := store.GetReport(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "report_not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (a API) log(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	store, ok := a.Store.(interface {
		GetLog(context.Context, string) ([]byte, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}
	body, err := store.GetLog(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "log_not_found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(body)
}

func resolveRef(ctx context.Context, c *http.Client, owner, name, kind, value string) (string, error) {
	base := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	var ref struct {
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := githubGet(ctx, c, base+"/git/ref/"+kind+"/"+value, &ref); err != nil {
		return "", err
	}
	for depth := 0; ref.Object.Type == "tag" && depth < 5; depth++ {
		var tag struct {
			Object struct {
				SHA  string `json:"sha"`
				Type string `json:"type"`
			} `json:"object"`
		}
		if err := githubGet(ctx, c, base+"/git/tags/"+ref.Object.SHA, &tag); err != nil {
			return "", err
		}
		ref.Object = tag.Object
	}
	if ref.Object.Type != "commit" || !fullSHA.MatchString(ref.Object.SHA) {
		return "", errors.New("ref does not resolve to a commit")
	}
	return ref.Object.SHA, nil
}
func repoParts(repo string) (string, string, bool) {
	p := strings.Split(strings.TrimPrefix(repo, "https://github.com/"), "/")
	if len(p) != 2 {
		return "", "", false
	}
	return p[0], strings.TrimSuffix(p[1], ".git"), true
}
func validRef(ref string) bool {
	if len(ref) == 0 || len(ref) > 255 || strings.HasPrefix(ref, "-") || strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") || strings.Contains(ref, "@") {
		return false
	}
	for _, c := range ref {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._/", c)) {
			return false
		}
	}
	for _, part := range strings.Split(ref, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
func githubGet(ctx context.Context, c *http.Client, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "RepoTest-local/0.1")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubStatusError{resp.StatusCode, resp.Header.Get("X-RateLimit-Reset")}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
}

type githubStatusError struct {
	status int
	reset  string
}

func (e githubStatusError) Error() string { return "GitHub API request failed" }
func writeGitHubError(w http.ResponseWriter, err error) {
	var ge githubStatusError
	if errors.As(err, &ge) {
		switch ge.status {
		case 404:
			writeError(w, 404, "repo_or_ref_not_found")
		case 403, 429:
			writeJSON(w, 429, map[string]any{"error": map[string]string{"code": "github_rate_limited"}, "retry_after": ge.reset})
		default:
			writeError(w, 502, "github_unavailable")
		}
		return
	}
	writeError(w, 502, "github_unavailable")
}
func (a API) get(w http.ResponseWriter, r *http.Request) {
	t, err := a.Store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		writeError(w, 404, "task_not_found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	writeJSON(w, 200, t)
}
func (a API) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.cancelTask(r.Context(), id); err != nil {
		writeTaskMutationError(w, err)
		return
	}
	task, err := a.Store.Get(r.Context(), id)
	if err != nil {
		writeTaskMutationError(w, err)
		return
	}
	if task.Status != domain.StatusCancelled {
		writeError(w, http.StatusConflict, "task_not_cancelled")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a API) cancelTask(ctx context.Context, id string) error {
	task, err := a.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	switch task.Status {
	case domain.StatusQueued:
		changed, transitionErr := a.Store.Transition(ctx, id, domain.StatusQueued, domain.StatusCancelled, "")
		if transitionErr != nil || changed {
			return transitionErr
		}
		_, err = a.Store.Get(ctx, id)
		return err
	case domain.StatusRunning:
		if a.Processes == nil {
			return ErrTaskProcessNotRegistered
		}
		if err = a.Processes.CancelAndWait(ctx, id); err != nil {
			return err
		}
		changed, transitionErr := a.Store.Transition(ctx, id, domain.StatusRunning, domain.StatusCancelled, "")
		if transitionErr != nil || changed {
			return transitionErr
		}
		_, err = a.Store.Get(ctx, id)
		return err
	default:
		return nil
	}
}

func writeTaskMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "task_not_found")
	case errors.Is(err, ErrTaskProcessNotRegistered), errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "task_not_cancellable")
	case errors.Is(err, ErrPreviewInvalid):
		writeError(w, http.StatusBadRequest, "invalid_preview")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "process_reap_timeout")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}
func (a API) delete(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "local_only")
		return
	}
	id := r.PathValue("id")
	task, err := a.Store.Get(r.Context(), id)
	if err != nil {
		writeTaskMutationError(w, err)
		return
	}
	if task.Status == domain.StatusQueued || task.Status == domain.StatusRunning {
		if err = a.cancelTask(r.Context(), id); err != nil {
			writeTaskMutationError(w, err)
			return
		}
		task, err = a.Store.Get(r.Context(), id)
		if err != nil {
			writeTaskMutationError(w, err)
			return
		}
		if task.Status == domain.StatusQueued || task.Status == domain.StatusRunning {
			writeError(w, http.StatusConflict, "task_still_active")
			return
		}
	}
	if err = a.Store.Delete(r.Context(), id); err != nil {
		writeTaskMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
