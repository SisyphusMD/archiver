package hostguard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPermits(t *testing.T) {
	own, _ := os.Hostname()
	allowed := Allowed("status.example.com, Backup.LAN")
	for host, want := range map[string]bool{
		"192.168.1.5:8470":        true,
		"[::1]:8470":              true,
		"127.0.0.1":               true,
		"localhost:8470":          true,
		own + ":8470":             true,
		"status.example.com":      true,
		"backup.lan:8470":         true,
		"BACKUP.LAN.":             true,
		"attacker.example:8470":   false,
		"status.example.com.evil": false,
		"":                        false,
		"evil.status.example.com": false,
	} {
		if got := Permits(host, allowed); got != want {
			t.Errorf("%q: got %v, want %v", host, got, want)
		}
	}
}

func TestGuard(t *testing.T) {
	h := Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }), nil)
	for host, code := range map[string]int{"10.0.0.2:8470": 200, "rebound.example:8470": http.StatusMisdirectedRequest} {
		r := httptest.NewRequest(http.MethodGet, "/status.json", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != code {
			t.Errorf("%s: %d, want %d", host, w.Code, code)
		}
	}
}
