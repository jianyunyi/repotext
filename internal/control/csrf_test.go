package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoopbackHomeIssuesNonEmptyCSRFToken(t *testing.T) {
	req := loopbackRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	res := httptest.NewRecorder()
	API{}.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	start := strings.Index(body, `name="repotest-csrf" content="`)
	if start < 0 {
		t.Fatal("CSRF token meta tag missing")
	}
	token := body[start+len(`name="repotest-csrf" content="`):]
	end := strings.IndexByte(token, '"')
	if end <= 0 {
		t.Fatal("CSRF token is empty")
	}
}

func TestLoopbackPreviewRejectsMissingCSRFToken(t *testing.T) {
	req := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget"}`)))
	req.Header.Del("X-CSRF-Token")
	res := httptest.NewRecorder()
	API{}.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("missing token status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestLoopbackMutationRejectsIncorrectCSRFToken(t *testing.T) {
	req := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/previews", strings.NewReader(`{"repo_url":"https://github.com/acme/widget"}`)))
	req.Header.Del("X-CSRF-Token")
	req.Header.Set("X-CSRF-Token", "invalid")
	res := httptest.NewRecorder()
	API{}.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("incorrect token status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestLoopbackPreviewAcceptsIssuedCSRFToken(t *testing.T) {
	req := loopbackRequest(httptest.NewRequest(http.MethodPost, "/api/v1/previews", strings.NewReader(`{"repo_url":"http://example.com/repo"}`)))
	res := httptest.NewRecorder()
	API{CSRFToken: testCSRFToken}.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("valid token did not reach preview validation: status=%d body=%s", res.Code, res.Body.String())
	}
}
