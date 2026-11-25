package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"repotest/internal/domain"
)

func TestLocalSessionManagementRevokesOnlySelectedDevice(t *testing.T) {
	store := authStore(t)
	pairing, code, err := NewPairing(store, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	first, err := pairing.Consume(code, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	otherPairing, otherCode, err := NewPairing(store, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := otherPairing.Consume(otherCode, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expired := sha256.Sum256([]byte("expired-device"))
	if err := store.CreateSession(context.Background(), expired[:], time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "session-task", Status: domain.StatusSucceeded, CreatedAt: time.Now()}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReport(context.Background(), task.ID, []byte(`{"packages":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.PutLog(context.Background(), task.ID, []byte("output")); err != nil {
		t.Fatal(err)
	}
	local := (API{Store: store, CSRFToken: testCSRFToken}).Handler()
	request := loopbackRequest(httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil))
	response := httptest.NewRecorder()
	local.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session metadata must not be cached")
	}
	if strings.Contains(response.Body.String(), first) || strings.Contains(response.Body.String(), second) {
		t.Fatal("session list leaked bearer token")
	}
	var sessions []struct {
		ID        string    `json:"id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("active sessions=%d", len(sessions))
	}
	firstHash := sha256.Sum256([]byte(first))
	firstID := hex.EncodeToString(firstHash[:])
	found := false
	for _, session := range sessions {
		if len(session.ID) != 64 || !session.ExpiresAt.After(time.Now()) {
			t.Fatalf("invalid public session metadata: %+v", session)
		}
		found = found || session.ID == firstID
	}
	if !found {
		t.Fatal("selected session handle missing")
	}
	for i := 0; i < 2; i++ {
		request = loopbackRequest(httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+firstID, nil))
		response = httptest.NewRecorder()
		local.ServeHTTP(response, request)
		if response.Code != 204 {
			t.Fatalf("revoke status=%d body=%s", response.Code, response.Body.String())
		}
	}
	lan := NewLANHandler(API{Store: store}, pairing, []string{"127.0.0.1"})
	for _, path := range []string{"/api/v1/tasks/session-task", "/api/v1/tasks/session-task/report", "/api/v1/tasks/session-task/log"} {
		for _, device := range []struct {
			token string
			want  int
		}{{first, 401}, {second, 200}, {firstID, 401}} {
			request = httptest.NewRequest(http.MethodGet, path, nil)
			request.Host = "127.0.0.1:8080"
			request.AddCookie(&http.Cookie{Name: "repotest_read", Value: device.token})
			response = httptest.NewRecorder()
			lan.ServeHTTP(response, request)
			if response.Code != device.want {
				t.Fatalf("read %s status=%d want=%d", path, response.Code, device.want)
			}
		}
	}
}

func TestLocalSessionManagementRejectsUntrustedRequests(t *testing.T) {
	store := authStore(t)
	local := (API{Store: store, CSRFToken: testCSRFToken}).Handler()
	id := strings.Repeat("a", 64)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/api/v1/sessions"
		if method == http.MethodDelete {
			path += "/" + id
		}
		for _, attack := range []string{"missing csrf", "wrong csrf", "LAN remote", "wrong host", "wrong origin"} {
			t.Run(method+"/"+attack, func(t *testing.T) {
				request := loopbackRequest(httptest.NewRequest(method, path, nil))
				switch attack {
				case "missing csrf":
					request.Header.Del("X-CSRF-Token")
				case "wrong csrf":
					request.Header.Set("X-CSRF-Token", "wrong")
				case "LAN remote":
					request.RemoteAddr = "192.168.1.20:1234"
				case "wrong host":
					request.Host = "attacker.example:8080"
				case "wrong origin":
					request.Header.Set("Origin", "http://attacker.example")
				}
				response := httptest.NewRecorder()
				local.ServeHTTP(response, request)
				if response.Code != 403 {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestLocalSessionManagementEmptyInvalidAndUnavailable(t *testing.T) {
	store := authStore(t)
	local := (API{Store: store, CSRFToken: testCSRFToken}).Handler()
	response := httptest.NewRecorder()
	local.ServeHTTP(response, loopbackRequest(httptest.NewRequest("GET", "/api/v1/sessions", nil)))
	if response.Code != 200 || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("empty list: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, id := range []string{"short", strings.Repeat("z", 64)} {
		response = httptest.NewRecorder()
		local.ServeHTTP(response, loopbackRequest(httptest.NewRequest("DELETE", "/api/v1/sessions/"+id, nil)))
		if response.Code != 400 {
			t.Fatalf("invalid id status=%d", response.Code)
		}
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/api/v1/sessions"}, {"DELETE", "/api/v1/sessions/" + strings.Repeat("a", 64)}} {
		response = httptest.NewRecorder()
		(API{CSRFToken: testCSRFToken}).Handler().ServeHTTP(response, loopbackRequest(httptest.NewRequest(tc.method, tc.path, nil)))
		if response.Code != 503 {
			t.Fatalf("no store status=%d", response.Code)
		}
	}
}
