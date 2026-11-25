package control

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type previewRoundTripper func(*http.Request) (*http.Response, error)

func (f previewRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestPreviewReturnsResolvedPublicRepoWithoutCreatingTask(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/repos/acme/widget") {
			return jsonResponse(200, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":512}`), nil
		}
		return jsonResponse(200, `{"object":{"sha":"0123456789012345678901234567890123456789","type":"commit"}}`), nil
	})}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"main"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestPreviewRejectsInvalidURLBeforeGitHubRequest(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: previewRoundTripper(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com.evil.test/a/b"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 || calls != 0 {
		t.Fatalf("status=%d calls=%d", rec.Code, calls)
	}
}
func TestPreviewRejectsMalformedRef(t *testing.T) {
	for _, ref := range []string{"../main", "feature#x", "bad ref", "-option"} {
		req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"`+ref+`"}`)))
		rec := httptest.NewRecorder()
		API{CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Errorf("ref=%q status=%d", ref, rec.Code)
		}
	}
}
func TestPreviewResolvesDefaultBranchAndVerifiesSHA(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos/acme/widget"):
			return jsonResponse(200, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"trunk","size":2}`), nil
		case strings.HasSuffix(r.URL.Path, "/git/ref/heads/trunk"):
			return jsonResponse(200, `{"object":{"sha":"0123456789012345678901234567890123456789","type":"commit"}}`), nil
		case strings.HasSuffix(r.URL.Path, "/git/commits/0123456789012345678901234567890123456789"):
			return jsonResponse(200, `{"sha":"0123456789012345678901234567890123456789"}`), nil
		}
		t.Fatalf("unexpected URL %s", r.URL)
		return nil, nil
	})}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestPreviewTargetArchitectureMatchesServiceRuntime(t *testing.T) {
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/repos/acme/widget") {
			return jsonResponse(200, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":1}`), nil
		}
		return jsonResponse(200, `{"object":{"sha":"0123456789012345678901234567890123456789","type":"commit"}}`), nil
	})}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"goarch":"amd64"`) {
		t.Fatalf("wrong target: %s", rec.Body.String())
	}
}
