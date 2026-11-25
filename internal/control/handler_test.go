package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomeServesPreviewFormAndNoExecutionAction(t *testing.T) {
	rec := httptest.NewRecorder()
	API{CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, loopbackRequest(httptest.NewRequest("GET", "/", nil)))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "/api/v1/previews") || !strings.Contains(body, "EXECUTION DISABLED") || strings.Contains(body, "/api/v1/tasks\"") {
		t.Fatalf("status=%d page missing preview-only contract", rec.Code)
	}
}
func TestUnknownStaticPathIsNotServed(t *testing.T) {
	rec := httptest.NewRecorder()
	API{CSRFToken: testCSRFToken}.Handler().ServeHTTP(rec, loopbackRequest(httptest.NewRequest("GET", "/missing.js", nil)))
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}

const testCSRFToken = "test-csrf-token"

func loopbackRequest(r *http.Request) *http.Request {
	r.RemoteAddr = "127.0.0.1:12345"
	r.Host = "127.0.0.1:8080"
	r.Header.Set("X-CSRF-Token", testCSRFToken)
	return r
}
