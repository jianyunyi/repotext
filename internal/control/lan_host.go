package control

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
)

func LANAllowedHosts(listener net.Listener) ([]string, error) {
	if listener == nil {
		return nil, fmt.Errorf("LAN listener is nil")
	}
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("LAN listener is not TCP")
	}
	if tcp.IP != nil && !tcp.IP.IsUnspecified() {
		return []string{tcp.IP.String()}, nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, iface := range interfaces {
		addrs, e := iface.Addrs()
		if e != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}
			set[ip.String()] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no concrete interface IPs for LAN listener")
	}
	hosts := make([]string, 0, len(set))
	for host := range set {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts, nil
}

func lanHostAllowed(hostport string, allowlist []string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	} else if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		host = strings.Trim(hostport, "[]")
	}
	if zone := strings.LastIndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	normalized := ip.String()
	for _, allowed := range allowlist {
		if parsed := net.ParseIP(allowed); parsed != nil && parsed.String() == normalized {
			return true
		}
	}
	return false
}

func lanHostOnly(next http.Handler, allowlist []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !lanHostAllowed(r.Host, allowlist) {
			writeError(w, http.StatusMisdirectedRequest, "host_not_allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}
