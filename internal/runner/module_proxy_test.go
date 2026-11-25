package runner

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type moduleProxyRoundTripper func(*http.Request) (*http.Response, error)

func (f moduleProxyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestModuleProxyForwardsModuleAndSumDBPaths(t *testing.T) {
	var urls []string
	client := &http.Client{Transport: moduleProxyRoundTripper(func(r *http.Request) (*http.Response, error) {
		urls = append(urls, r.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	proxy := newModuleProxy("https://proxy.golang.org", client, 64, 128)
	for _, target := range []string{"/github.com/acme/mod/@v/v1.2.3.mod", "/sumdb/sum.golang.org/supported"} {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
			t.Fatalf("target=%q status=%d body=%q", target, rec.Code, rec.Body.String())
		}
	}
	if len(urls) != 2 || !strings.HasPrefix(urls[0], "https://proxy.golang.org/") || !strings.HasPrefix(urls[1], "https://proxy.golang.org/sumdb/") {
		t.Fatalf("unexpected upstream requests: %v", urls)
	}
}

func TestModuleProxyRejectsUnsafeRequestsBeforeUpstream(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: moduleProxyRoundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, io.EOF
	})}
	proxy := newModuleProxy("https://proxy.golang.org", client, 64, 128)
	for _, tc := range []struct{ method, target string }{
		{http.MethodPost, "/github.com/acme/mod/@v/v1.2.3.mod"},
		{http.MethodGet, "/../secret"},
		{http.MethodGet, "/%2e%2e/secret"},
		{http.MethodGet, "/%252e%252e/secret"},
		{http.MethodGet, "/github.com/acme/mod/@v/v1.2.3.mod?next=https://evil.test"},
		{http.MethodGet, "/unrecognized/path"},
	} {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
		if rec.Code < 400 {
			t.Errorf("accepted %s %q: %d", tc.method, tc.target, rec.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("unsafe requests reached upstream %d times", calls)
	}
}

func TestModuleProxyAllowsOnlyStorageRedirectForZipRequests(t *testing.T) {
	for _, tc := range []struct {
		location string
		allowed  bool
	}{
		{"https://storage.googleapis.com/proxy-golang-org-prod/github.com/acme/mod/@v/v1.2.3.zip?sig=x", true},
		{"http://storage.googleapis.com/proxy-golang-org-prod/github.com/acme/mod/@v/v1.2.3.zip", false},
		{"https://storage.googleapis.com.evil.test/proxy-golang-org-prod/github.com/acme/mod/@v/v1.2.3.zip", false},
		{"https://example.com/mod.zip", false},
		{"https://storage.googleapis.com/other/object", false},
		{"https://user@storage.googleapis.com/proxy-golang-org-prod/github.com/acme/mod/@v/v1.2.3.zip", false},
	} {
		if got := allowedModuleProxyRedirect("/github.com/acme/mod/@v/v1.2.3.zip", tc.location); got != tc.allowed {
			t.Errorf("allowedModuleProxyRedirect(%q)=%v, want %v", tc.location, got, tc.allowed)
		}
	}
	if allowedModuleProxyRedirect("/github.com/acme/mod/@v/v1.2.3.mod", "https://storage.googleapis.com/proxy-golang-org-prod/mod.zip") {
		t.Fatal("non-zip request accepted storage redirect")
	}
}

func TestModuleProxyBoundsBodyAndAggregateBytes(t *testing.T) {
	client := &http.Client{Transport: moduleProxyRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("0123456789"))}, nil
	})}
	proxy := newModuleProxy("https://proxy.golang.org", client, 12, 16)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/github.com/acme/mod/@v/v1.2.3.zip", nil))
		if i == 0 && (rec.Code != http.StatusOK || rec.Body.String() != "0123456789") {
			t.Fatalf("first request status=%d body=%q", rec.Code, rec.Body.String())
		}
		if i == 1 && rec.Code != http.StatusBadGateway {
			t.Fatalf("request %d status=%d body=%q", i, rec.Code, rec.Body.String())
		}
	}
	if got := proxy.transferred.Load(); got != 10 {
		t.Fatalf("proxy reserved %d bytes before denying second response", got)
	}
}

func TestPublicModuleProxyAddressRejectsPrivateAndReservedRanges(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "240.0.0.1", "::1", "fd00::1", "fe80::1"} {
		addr := netip.MustParseAddr(raw)
		if isPublicModuleProxyAddress(addr) {
			t.Errorf("accepted non-public address %s", addr)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		addr := netip.MustParseAddr(raw)
		if !isPublicModuleProxyAddress(addr) {
			t.Errorf("rejected public address %s", addr)
		}
	}
}

func TestModuleProxyRedirectDoesNotFollowSecondHop(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: moduleProxyRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://storage.googleapis.com/proxy-golang-org-prod/github.com/acme/mod/@v/v1.2.3.zip?sig=first"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://example.com/file.zip"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	proxy := newModuleProxy("https://proxy.golang.org", client, 64, 128)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/github.com/acme/mod/@v/v1.2.3.zip", nil))
	if calls != 2 || rec.Code < 400 {
		t.Fatalf("calls=%d status=%d body=%q", calls, rec.Code, rec.Body.String())
	}
}

func TestModuleProxyDialerRejectsHostOutsideFixedAllowlist(t *testing.T) {
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	if _, err := publicModuleProxyDialer(lookup).DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("dialer accepted a host outside its fixed allowlist")
	}
}

func TestModuleProxyDialerRejectsMixedPublicAndPrivateDNSAnswers(t *testing.T) {
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}, nil
	}
	if _, err := publicModuleProxyDialer(lookup).DialContext(context.Background(), "tcp", "proxy.golang.org:443"); err == nil {
		t.Fatal("dialer accepted a mixed public/private DNS answer")
	}
}

func TestModuleProxyDialerPinsConnectionToValidatedIP(t *testing.T) {
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	var network, address string
	dialer := publicModuleProxyDialer(lookup)
	dialer.dialContext = func(_ context.Context, networkName, addr string) (net.Conn, error) {
		network, address = networkName, addr
		return nil, io.EOF
	}
	_, _ = dialer.DialContext(context.Background(), "tcp", "proxy.golang.org:443")
	if network != "tcp" || address != "8.8.8.8:443" {
		t.Fatalf("dial target=%s %s", network, address)
	}
}

func TestModuleProxyQueuesConcurrentModuleDownloads(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	client := &http.Client{Transport: moduleProxyRoundTripper(func(*http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), ContentLength: 2}, nil
	})}
	proxy := newModuleProxy("https://proxy.golang.org", client, 64, 128)
	responses := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func(i int) {
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/github.com/acme/mod/@v/v1.2.3.mod", nil))
			responses <- rec.Code
		}(i)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("proxy did not start two bounded concurrent requests")
		}
	}
	close(release)
	for i := 0; i < 3; i++ {
		if status := <-responses; status != http.StatusOK {
			t.Fatalf("concurrent response status=%d", status)
		}
	}
}
