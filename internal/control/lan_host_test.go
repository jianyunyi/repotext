package control

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLANHandlerRejectsHostOutsideBoundInterfaceAllowlist(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	allowed, err := LANAllowedHosts(ln)
	if err != nil {
		t.Fatal(err)
	}
	h := NewLANHandler(API{}, nil, allowed)
	for _, tc := range []struct {
		host string
		want int
	}{{"127.0.0.1:29192", http.StatusOK}, {"attacker.example:29192", http.StatusMisdirectedRequest}, {"localhost:29192", http.StatusMisdirectedRequest}} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Host = tc.host
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Errorf("Host %q status=%d want=%d", tc.host, res.Code, tc.want)
		}
	}
}

func TestLANAllowedHostsUsesExplicitBindingAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hosts, err := LANAllowedHosts(ln)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0] != "127.0.0.1" {
		t.Fatalf("hosts=%v", hosts)
	}
}

func lanTestRequest(r *http.Request) *http.Request { r.Host = "127.0.0.1:8080"; return r }

func TestLANAllowedHostsForWildcardUsesConcreteInterfaceAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("wildcard bind unavailable: %v", err)
	}
	defer ln.Close()
	hosts, err := LANAllowedHosts(ln)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) == 0 {
		t.Fatal("no concrete interface addresses returned")
	}
	for _, host := range hosts {
		if net.ParseIP(host) == nil {
			t.Fatalf("non-IP host in allowlist: %q", host)
		}
	}
}
