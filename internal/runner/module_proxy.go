package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"net/netip"
)

const (
	moduleProxyMaxResponse = 100 << 20
	moduleProxyMaxTotal    = 512 << 20
)

var moduleProxyHosts = map[string]struct{}{
	"proxy.golang.org":       {},
	"storage.googleapis.com": {},
}

type moduleIPLookup func(context.Context, string) ([]netip.Addr, error)

type moduleProxy struct {
	upstream    *url.URL
	client      *http.Client
	maxBody     int64
	maxTotal    int64
	transferred atomic.Int64
	requests    chan struct{}
}

func newModuleProxy(upstream string, client *http.Client, maxBody, maxTotal int64) *moduleProxy {
	base, _ := url.Parse(upstream)
	if maxBody <= 0 {
		maxBody = moduleProxyMaxResponse
	}
	if maxTotal <= 0 {
		maxTotal = moduleProxyMaxTotal
	}
	return &moduleProxy{upstream: base, client: client, maxBody: maxBody, maxTotal: maxTotal, requests: make(chan struct{}, 2)}
}

func newPublicModuleProxy() *moduleProxy {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: publicModuleProxyDialer(func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		DisableCompression:    true,
	}
	return newModuleProxy("https://proxy.golang.org", &http.Client{Transport: transport, Timeout: 45 * time.Second}, moduleProxyMaxResponse, moduleProxyMaxTotal)
}

func (p *moduleProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.ContentLength > 0 || len(r.TransferEncoding) > 0 || r.URL.RawQuery != "" || !validModuleProxyPath(r.URL.EscapedPath()) {
		http.Error(w, "module proxy request rejected", http.StatusBadRequest)
		return
	}
	select {
	case p.requests <- struct{}{}:
		defer func() { <-p.requests }()
	case <-r.Context().Done():
		return
	}

	endpoint := *p.upstream
	endpoint.Path = r.URL.Path
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		http.Error(w, "module proxy request rejected", http.StatusBadRequest)
		return
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "RepoTest-module-proxy/0.1")
	client := *p.client
	client.Jar = nil
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) != 1 || !allowedModuleProxyRedirect(via[0].URL.EscapedPath(), next.URL.String()) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "module proxy upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		http.Error(w, "module proxy redirect rejected", http.StatusBadGateway)
		return
	}
	if resp.ContentLength > p.maxBody {
		http.Error(w, "module response exceeds limit", http.StatusBadGateway)
		return
	}
	body := &bytes.Buffer{}
	written, err := io.Copy(&moduleBudgetWriter{buffer: body, proxy: p}, io.LimitReader(resp.Body, p.maxBody+1))
	if err != nil || written > p.maxBody {
		http.Error(w, "module response exceeds limit", http.StatusBadGateway)
		return
	}
	for _, name := range []string{"Content-Type", "Last-Modified", "ETag"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body.Bytes())
}

func (p *moduleProxy) reserveBytes(n int64) bool {
	for {
		old := p.transferred.Load()
		if n < 0 || n > p.maxTotal-old {
			return false
		}
		if p.transferred.CompareAndSwap(old, old+n) {
			return true
		}
	}
}

func validModuleProxyPath(raw string) bool {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, "\\\x00\r\n") || strings.Contains(strings.ToLower(raw), "%2f") || strings.Contains(strings.ToLower(raw), "%5c") || strings.Contains(strings.ToLower(raw), "%2e") {
		return false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.Contains(strings.ToLower(decoded), "%2f") || strings.Contains(strings.ToLower(decoded), "%5c") || strings.Contains(strings.ToLower(decoded), "%2e") || !strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "//") || path.Clean(decoded) != decoded {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(decoded, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\x00\r\n") {
			return false
		}
	}
	if strings.HasPrefix(decoded, "/sumdb/sum.golang.org/") {
		return true
	}
	if strings.HasSuffix(decoded, "/@latest") {
		return len(strings.TrimPrefix(strings.TrimSuffix(decoded, "/@latest"), "/")) > 0
	}
	parts := strings.Split(strings.TrimPrefix(decoded, "/"), "/")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "@v" && i+1 == len(parts)-1 {
			v := parts[i+1]
			return strings.HasSuffix(v, ".info") || strings.HasSuffix(v, ".mod") || strings.HasSuffix(v, ".zip") || v == "list"
		}
	}
	return false
}

func allowedModuleProxyRedirect(sourcePath, rawLocation string) bool {
	if !strings.HasSuffix(sourcePath, ".zip") || !validModuleProxyPath(sourcePath) {
		return false
	}
	u, err := url.Parse(rawLocation)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "storage.googleapis.com") || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawQuery == "" {
		return false
	}
	if u.EscapedPath() != "/proxy-golang-org-prod"+sourcePath || path.Clean(u.Path) != u.Path {
		return false
	}
	return true
}

func publicModuleProxyDialer(lookup moduleIPLookup) *validatedModuleDialer {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &validatedModuleDialer{lookup: lookup, dialContext: dialer.DialContext}
}

func isPublicModuleProxyAddress(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	reserved := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "fec0::/10",
	}
	for _, raw := range reserved {
		prefix := netip.MustParsePrefix(raw)
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

type validatedModuleDialer struct {
	lookup      moduleIPLookup
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func (d validatedModuleDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("module proxy permits TCP only")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("module proxy permits HTTPS destinations only")
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, ok := moduleProxyHosts[host]; !ok {
		return nil, errors.New("module proxy destination is not allowlisted")
	}
	ips, err := d.lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("module proxy destination resolution failed")
	}
	for _, ip := range ips {
		if !isPublicModuleProxyAddress(ip) {
			return nil, fmt.Errorf("module proxy destination resolved to a non-public address")
		}
	}
	return d.dialContext(ctx, "tcp", net.JoinHostPort(ips[0].String(), "443"))
}

type moduleBudgetWriter struct {
	buffer *bytes.Buffer
	proxy  *moduleProxy
}

func (w *moduleBudgetWriter) Write(data []byte) (int, error) {
	if !w.proxy.reserveBytes(int64(len(data))) {
		return 0, errors.New("module proxy aggregate byte limit exceeded")
	}
	return w.buffer.Write(data)
}
