package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewMapsMissingCommitSHAToNotFound(t *testing.T) {
	client := &http.Client{Transport: previewRoundTripper(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/repos/acme/widget") {
			return jsonResponse(200, `{"full_name":"acme/widget","private":false,"archived":false,"default_branch":"main","size":1}`), nil
		}
		return jsonResponse(422, `{"message":"No commit found for SHA"}`), nil
	})}
	req := loopbackRequest(httptest.NewRequest("POST", "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget","ref":"0123456789012345678901234567890123456789"}`)))
	rec := httptest.NewRecorder()
	API{GitHubClient: client, CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "repo_or_ref_not_found") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
