package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLANHandlerAllowsOnlyReadRoutes(t *testing.T) {
	h := NewLANHandler(API{Store: NewMemoryStore()}, nil, []string{"127.0.0.1"})
	for _, tc := range []struct {
		method, path string
		want         int
	}{{http.MethodPost, "/api/v1/tasks", 405}, {http.MethodPost, "/api/v1/previews", 405}, {http.MethodDelete, "/api/v1/tasks/x", 405}, {http.MethodGet, "/api/v1/tasks/x", 401}, {http.MethodGet, "/admin", 404}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s status=%d want=%d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}
