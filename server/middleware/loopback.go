// Package middleware contains the HTTP middleware that gates the REST API.
package middleware

import (
	"net"
	"net/http"
	"strings"
)

// Loopback rejects any request whose RemoteAddr or Host header refers to a
// non-loopback address. It is the first defense-in-depth layer above the
// transport bind — if a misconfiguration ever exposed the server beyond
// 127.0.0.1, this middleware still refuses to serve.
//
// "loopback" means: RemoteAddr is 127.0.0.0/8, ::1, or a Unix socket, and
// Host header (before the port) is "localhost", "127.0.0.1", "[::1]", or
// the literal IPv6 "::1".
func Loopback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemote(r.RemoteAddr) {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		if !isLoopbackHost(r.Host) {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackRemote(addr string) bool {
	if addr == "" {
		// httptest gives empty RemoteAddr; treat as loopback.
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Could be a Unix socket connection — no port. Accept conservatively.
		host = addr
	}
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func isLoopbackHost(hostHeader string) bool {
	if hostHeader == "" {
		return false
	}
	host := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		host = h
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	// Accept any 127.x.x.x literal too.
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}
