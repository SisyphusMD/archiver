package harness

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// SignalDir is where a deployment sees the host directory given to Mount for hook
// signalling, so hooks and tests can hand each other marker files.
const SignalDir = "/e2e-signal"

// SendSIGTERM delivers SIGTERM to the container and returns at once, without the SIGKILL
// that `docker stop` escalates to, so a container that ignores it is caught by
// WaitStopped instead of being killed out from under the test.
func (d *Deployment) SendSIGTERM(t testing.TB) {
	t.Helper()
	if r := docker(t, "kill", "--signal", "TERM", d.name); r.Code != 0 {
		t.Fatalf("SIGTERM %s: %s", d.name, r.Output())
	}
}

// WaitStopped polls until the container is no longer running and reports whether that
// happened within timeout.
func (d *Deployment) WaitStopped(t testing.TB, timeout time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(timeout); ; {
		r := docker(t, "inspect", "-f", "{{.State.Running}}", d.name)
		if r.Code == 0 && strings.TrimSpace(r.Stdout) == "false" {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// ServiceHooks are shell snippets a service runs before and after its backup.
type ServiceHooks struct{ Pre, Post string }

// InstallServiceHooks gives the service at serviceDir its hooks. The 0.11 form, functions
// in a sourced service-backup-settings.sh, is incidental (ADR 1): v1 may make hooks
// executables, and only this function changes.
func InstallServiceHooks(t testing.TB, serviceDir string, h ServiceHooks) {
	t.Helper()
	body := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "  :"
		}
		return s
	}
	src := fmt.Sprintf("service_specific_pre_backup_function() {\n%s\n}\n\nservice_specific_post_backup_function() {\n%s\n}\n",
		body(h.Pre), body(h.Post))
	if err := os.WriteFile(filepath.Join(serviceDir, "service-backup-settings.sh"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// BreakStorage makes a local storage unable to take or give chunks while its config and
// snapshot files stay readable, so a run gets far enough to try and then must fail.
func BreakStorage(t testing.TB, storageDir string) {
	t.Helper()
	chunks := filepath.Join(storageDir, "chunks")
	if err := os.RemoveAll(chunks); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunks, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Notifier is a fake notification service that counts what a deployment sends it.
type Notifier struct {
	addr    string
	caPEM   []byte
	token   string
	user    string
	mu      sync.Mutex
	arrived int
}

// NewNotifier starts the fake on this runner, reachable from sibling containers.
//
// The 0.11 notifier is Pushover, whose endpoint is hardcoded to https://api.pushover.net,
// so the fake impersonates that host with a certificate from a throwaway CA; NotifyTo
// points the deployment's name resolution and trust store at it. Pushover is incidental
// (ADR 1): when v1 takes a configurable notifier, this becomes an ordinary local receiver.
func NewNotifier(t testing.TB) *Notifier {
	t.Helper()
	n := &Notifier{addr: runnerIP(t), token: "e2e-pushover-token", user: "e2e-pushover-user"}
	cert, caPEM := impersonate(t, "api.pushover.net")
	n.caPEM = caPEM

	mux := http.NewServeMux()
	mux.HandleFunc("/1/messages.json", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") == n.token && r.FormValue("user") == n.user && r.FormValue("message") != "" {
			n.mu.Lock()
			n.arrived++
			n.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":1,"request":"e2e"}`)
	})
	ln, err := net.Listen("tcp", net.JoinHostPort(n.addr, "443"))
	if err != nil {
		t.Fatalf("notifier listen: %v", err)
	}
	srv := &http.Server{Handler: mux, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}}
	go srv.ServeTLS(ln, "", "")
	t.Cleanup(func() { srv.Close() })
	return n
}

// NotifyTo configures d, before Start, to send its notifications to n and nowhere else:
// with no nameserver and only the fake's CA trusted, nothing can reach the real service.
func (d *Deployment) NotifyTo(t testing.TB, n *Notifier, dir string) {
	t.Helper()
	const inContainer = "/e2e-notifier"
	files := map[string]string{
		"pushover_api_token": n.token,
		"pushover_user_key":  n.user,
		"hosts":              "127.0.0.1 localhost " + d.Hostname + "\n" + n.addr + " api.pushover.net\n",
		"resolv.conf":        "nameserver 127.0.0.1\n",
		"ca.pem":             string(n.caPEM),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d.Mount(dir, inContainer)
	d.Mount(filepath.Join(dir, "hosts"), "/etc/hosts")
	d.Mount(filepath.Join(dir, "resolv.conf"), "/etc/resolv.conf")
	d.Mount(filepath.Join(dir, "ca.pem"), "/etc/ssl/certs/ca-certificates.crt")
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	d.Extra["NOTIFICATION_SERVICE"] = "pushover"
	d.Extra["PUSHOVER_API_TOKEN_FILE"] = inContainer + "/pushover_api_token"
	d.Extra["PUSHOVER_USER_KEY_FILE"] = inContainer + "/pushover_user_key"
}

// Secrets are the notifier credentials the deployment sends, by name.
func (n *Notifier) Secrets() map[string]string {
	return map[string]string{"Pushover API token": n.token, "Pushover user key": n.user}
}

// Arrived is how many notifications from the configured deployment n has received.
func (n *Notifier) Arrived() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.arrived
}

// runnerIP is this runner's address on the Docker network its sibling containers share.
func runnerIP(t testing.TB) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.String()
		}
	}
	t.Fatal("runner has no non-loopback IPv4 address")
	return ""
}

// impersonate issues a certificate for host from a fresh CA and returns it with the CA's
// PEM, which is all a client needs to trust it.
func impersonate(t testing.TB, host string) (tls.Certificate, []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "archiver e2e CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}
