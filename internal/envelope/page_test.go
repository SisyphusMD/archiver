package envelope

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

func page(t *testing.T, vars map[string]string, secrets map[string]string) *Page {
	t.Helper()
	d := t.TempDir()
	for k, v := range secrets {
		os.WriteFile(filepath.Join(d, strings.ToLower(k)), []byte(v), 0o600)
	}
	var env []string
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	s, err := config.Snapshot(config.Source{Getenv: func(k string) string { return vars[k] }, SecretsDir: d}, env)
	if err != nil {
		t.Fatal(err)
	}
	return Build(Source{Settings: s, Hostname: "h", SSHKeyFile: filepath.Join(d, "none"), QRFits: func(string) bool { return false }})
}

var b2 = map[string]string{"STORAGE_TARGET_1_NAME": "b", "STORAGE_TARGET_1_TYPE": "b2", "STORAGE_TARGET_1_B2_BUCKETNAME": "bkt"}

func TestBreakglassCredentialReplacesTheBackupOne(t *testing.T) {
	sec := map[string]string{"RECOVERY_PASSWORD": "recovery-pw", "STORAGE_TARGET_1_B2_ID": "id", "STORAGE_TARGET_1_B2_KEY": "key"}
	text := func(p *Page) string {
		var b strings.Builder
		for _, e := range p.Elements {
			b.WriteString(e.Kind + " " + e.Text + "\n")
		}
		return b.String()
	}
	backup := text(page(t, b2, sec))
	if !strings.Contains(backup, "warn FULL ACCESS") || !strings.Contains(backup, "m Application key: key\n") {
		t.Fatalf("backup credential page:\n%s", backup)
	}
	sec["STORAGE_TARGET_1_BREAKGLASS_B2_ID"], sec["STORAGE_TARGET_1_BREAKGLASS_B2_KEY"] = "bgid", "bgkey"
	bg := text(page(t, b2, sec))
	if strings.Contains(bg, "FULL ACCESS") || !strings.Contains(bg, "m Application key: bgkey\n") || strings.Contains(bg, "key: key\n") {
		t.Fatalf("break-glass page:\n%s", bg)
	}
}

func TestCheckAndConfirm(t *testing.T) {
	l := layout.Layout{Root: t.TempDir()}
	os.MkdirAll(l.LogDir(), 0o700)
	sec := map[string]string{"RECOVERY_PASSWORD": "recovery-pw", "STORAGE_TARGET_1_B2_ID": "id", "STORAGE_TARGET_1_B2_KEY": "key"}
	p := page(t, b2, sec)
	now := time.Unix(1_800_000_000, 0)
	if _, err := Confirm(l, p, now); !errors.Is(err, ErrNeverWritten) {
		t.Fatalf("confirm before writing: %v", err)
	}
	if s, err := Check(l, p, now); s != nil || err != nil {
		t.Fatalf("nothing confirmed: %v %v", s, err)
	}
	if _, err := Write(l, p, "h", filepath.Join(l.Root, "out"), now); err != nil {
		t.Fatal(err)
	}
	sec["STORAGE_TARGET_1_B2_KEY"] = "rotated"
	if _, err := Confirm(l, page(t, b2, sec), now); !errors.Is(err, ErrChanged) {
		t.Fatalf("confirm after a change: %v", err)
	}
	sec["STORAGE_TARGET_1_B2_KEY"] = "key"
	if fp, err := Confirm(l, page(t, b2, sec), now); err != nil || fp != p.Fingerprint() {
		t.Fatalf("confirm: %q %v", fp, err)
	}
	// Current: nothing. Changed: one notice, then quiet until it changes again.
	if s, _ := Check(l, page(t, b2, sec), now); s != nil {
		t.Fatalf("a current page: %+v", s)
	}
	sec["STORAGE_TARGET_1_B2_KEY"] = "rotated"
	if s, _ := Check(l, page(t, b2, sec), now); s == nil || s.Title != "Envelope Out of Date" {
		t.Fatalf("a changed page: %+v", s)
	}
	if s, _ := Check(l, page(t, b2, sec), now); s != nil {
		t.Fatal("notified twice for one change")
	}
	line, _ := Status(l, now, func(int64, time.Time) string { return "today" })
	if !strings.HasPrefix(line, "Envelope: OUT OF DATE") {
		t.Fatalf("status %q", line)
	}
	sec["STORAGE_TARGET_1_B2_KEY"] = "key"
	if s, _ := Check(l, page(t, b2, sec), now.Add(MaxAge+time.Hour)); s == nil || s.Title != "Envelope Check Due" {
		t.Fatalf("a year on: %+v", s)
	}
}

func TestWords(t *testing.T) {
	if got := strings.Join(words("  a  b\xa0c "), "|"); got != "a|b\xa0c" {
		t.Fatalf("%q", got)
	}
	if got := strings.Join(wrap("abcdefghij kl", 4), "|"); got != "abcd|efgh|ij|kl" {
		t.Fatalf("%q", got)
	}
}
