// Package hostguard keeps Archiver's HTTP endpoints (the status page, /metrics) from
// answering a request addressed to a name it does not know. They have no login, so what
// protects them is where they are reachable; a web page on another site could otherwise
// rebind its own name to Archiver's address and read them through a visitor's browser.
// An address needs no allowing: a page cannot read a response from an origin not its own.
package hostguard

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// Allowed lists the names a request may be addressed to besides addresses, localhost and
// the container's hostname: WEB_ALLOWED_HOSTS, separated by commas or spaces.
func Allowed(setting string) []string {
	return strings.FieldsFunc(strings.ToLower(setting), func(r rune) bool { return r == ',' || r == ' ' })
}

// Permits reports whether a request's Host header names this server.
func Permits(hostHeader string, allowed []string) bool {
	host := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil || host == "localhost" {
		return true
	}
	if own, err := os.Hostname(); err == nil && strings.EqualFold(host, own) {
		return true
	}
	for _, a := range allowed {
		if host == strings.TrimSuffix(a, ".") {
			return true
		}
	}
	return false
}

// Guard answers 421 to a request addressed to a name not permitted.
func Guard(h http.Handler, allowed []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Permits(r.Host, allowed) {
			http.Error(w, "this name is not one of Archiver's: add it to WEB_ALLOWED_HOSTS", http.StatusMisdirectedRequest)
			return
		}
		h.ServeHTTP(w, r)
	})
}
