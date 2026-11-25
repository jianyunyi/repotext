package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLANHomeProvidesPairingAndReadOnlyTaskPage(t *testing.T) {
	handler := NewLANHandler(API{CSRFToken: "local-secret"}, nil, []string{"127.0.0.1"})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "127.0.0.1:8080"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("home status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("LAN page must not be cached")
	}
	for _, required := range []string{`id="pair-form"`, `id="task-form"`, `id="pair-code"`, `id="task-id"`} {
		if !strings.Contains(response.Body.String(), required) {
			t.Fatalf("missing browser control %s", required)
		}
	}
	for _, secret := range []string{"local-secret", "repotest-csrf", "X-CSRF-Token", "confirmTask", "deleteTask", "cancelTask"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("LAN page contains management capability %s", secret)
		}
	}
}

func TestLANBrowserRoutesKeepHostAndMutationBoundaries(t *testing.T) {
	handler := NewLANHandler(API{}, nil, []string{"127.0.0.1"})
	for _, tc := range []struct {
		method, path, host string
		want               int
	}{
		{"GET", "/", "attacker.example", 421},
		{"GET", "/unknown", "127.0.0.1:8080", 404},
		{"GET", "/api/v1/tasks/unknown", "127.0.0.1:8080", 401},
		{"POST", "/api/v1/tasks", "127.0.0.1:8080", 405},
		{"DELETE", "/api/v1/tasks/unknown", "127.0.0.1:8080", 405},
		{"GET", "/api/v1/sessions", "127.0.0.1:8080", 405},
		{"DELETE", "/api/v1/sessions/unknown", "127.0.0.1:8080", 405},
	} {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request.Host = tc.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Errorf("%s %s host=%s status=%d want=%d", tc.method, tc.path, tc.host, response.Code, tc.want)
		}
	}
}
