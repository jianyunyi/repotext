package control

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func isLoopbackRequest(r *http.Request) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(remote) == nil || !net.ParseIP(remote).IsLoopback() {
		return false
	}
	requestHost, ok := loopbackHost(r.Host)
	if !ok {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		originHost, ok := loopbackHost(u.Host)
		if !ok || !strings.EqualFold(originHost, requestHost) || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}

func loopbackHost(hostport string) (string, bool) {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	} else if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		host = strings.Trim(hostport, "[]")
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", false
	}
	return ip.String(), true
}

func loopbackOnly(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) || token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(token)) != 1 {
			writeError(w, http.StatusForbidden, "local_only")
			return
		}
		next.ServeHTTP(w, r)
	})
}
