package envelope

import (
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Values with quotes survive both rclone's and the shell's quoting; a password is obscured;
// types reached otherwise get no command.
func TestFetchCommand(t *testing.T) {
	real := Obscure
	Obscure = func(p string) (string, error) { return "obscured(" + p + ")", nil }
	defer func() { Obscure = real }()
	got, err := fetchCommand("webdav", config.Values{"WEBDAV_HOST": "dav", "WEBDAV_USER": `o'brien`, "WEBDAV_PATH": "b", "WEBDAV_PASSWORD": `p"w`}, "kit.tar.enc")
	if err != nil {
		t.Fatal(err)
	}
	want := `rclone copyto ':webdav,pass="obscured(p""w)",url="https://dav",user="o'\''brien":/b/kit.tar.enc' kit.tar.enc`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	for _, typ := range []string{"local", "sftp", "sftpc", "one", "odb"} {
		if c, _ := fetchCommand(typ, config.Values{}, "k"); c != "" {
			t.Errorf("%s: %s", typ, c)
		}
	}
	c, _ := fetchCommand("gcs", config.Values{"GCS_BUCKETNAME": "b", "GCS_TOKEN": "{\n  \"type\": \"service_account\",\n\t\"private_key\": \"a\\nb\"\n}\n"}, "k")
	if !strings.HasPrefix(c, "rclone copyto ':gcs,") || strings.ContainsAny(c, "\n\t") || !strings.Contains(c, `""private_key"":""a\nb""`) {
		t.Errorf("gcs: %q", c)
	}
}

// A fetch command differs each time (a fresh obscured password), so it stays out of the
// fingerprint, which must not change while the page's content does not.
func TestFetchCommandOutOfFingerprint(t *testing.T) {
	a := &Page{password: "pw", Elements: []Element{{Kind: "m", Text: "x"}, {Kind: "cmd", Text: "one"}}}
	b := &Page{password: "pw", Elements: []Element{{Kind: "m", Text: "x"}, {Kind: "cmd", Text: "two"}}}
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("the fetch command changes the fingerprint")
	}
}
