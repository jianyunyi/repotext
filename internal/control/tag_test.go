package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewResolvesTagName(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case strings.HasSuffix(r.URL.Path, "/repos/acme/widget"):
			return jsonResponse(200, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":1}`), nil
		case strings.HasSuffix(r.URL.Path, "/git/ref/heads/v1.2.3"):
			return jsonResponse(404, `{"message":"Not Found"}`), nil
		case strings.HasSuffix(r.URL.Path, "/git/ref/tags/v1.2.3"):
			return jsonResponse(200, `{"object":{"sha":"0123456789012345678901234567890123456789","type":"commit"}}`), nil
		}
		t.Fatalf("unexpected URL %s", r.URL)
		return nil, nil
	})}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"v1.2.3"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "0123456789012345678901234567890123456789") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
