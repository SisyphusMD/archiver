package envelope

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Element is one element of the page. Kind is one of title, sub, h, p, m (monospace), warn,
// qr (Text is the encoded data), notes (Text is "label|line count") or rule.
type Element struct{ Kind, Text string }

// Page is the break-glass envelope: what a person needs to recover with no running system
// and no help from a storage's owner. The recovery password, then for each storage where
// the recovery kit sits a credential that can read it, with QR codes and the next steps.
type Page struct {
	Elements []Element
	password string
}

// Source is what a page is built from.
type Source struct {
	Settings   *config.Settings
	Hostname   string
	SSHKeyFile string // the backup's SSH private key, for an SFTP storage without a break-glass key
	// QRFits reports whether data fits in one QR code; nil asks qrencode.
	QRFits func(data string) bool
}

func (s Source) get(name string) string { v, _ := s.Settings.Get(name); return v }

func (s Source) qrFits(data string) bool {
	if s.QRFits != nil {
		return s.QRFits(data)
	}
	cmd := exec.Command("qrencode", "-o", os.DevNull)
	cmd.Stdin = strings.NewReader(data)
	return cmd.Run() == nil
}

// sshKey is the backup's SSH private key, or empty.
func (s Source) sshKey() string {
	b, err := os.ReadFile(s.SSHKeyFile)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

func (p *Page) add(kind, text string) { p.Elements = append(p.Elements, Element{kind, text}) }

// addQR adds a QR code of data, or, when it is too long for one, a note to type it from the
// text; it reports whether the code fit.
func (p *Page) addQR(s Source, data string) bool {
	if s.qrFits(data) {
		p.add("qr", data)
		return true
	}
	p.add("p", "Too long for a QR code: type it from the text.")
	return false
}

// Build lays out the page. Its fingerprint and wording are the bash envelope's: a page
// confirmed printed by either is current for both.
func Build(s Source) *Page {
	p := &Page{password: s.Settings.Secret("RECOVERY_PASSWORD")}
	kit := "archiver-recovery-kit-" + s.Hostname + ".tar.enc"
	p.add("title", "Break-glass envelope: "+s.Hostname)
	p.add("sub", "") // the date and fingerprint, filled in when written
	p.add("p", "Everything needed to recover "+s.Hostname+"'s backups with no running system: the recovery password unlocks the recovery kit, and any ONE storage below holds a copy of the kit and the backups. Keep this page sealed and somewhere that does not share fate with the backups.")
	p.add("h", "1. Recovery password")
	note := "Also in the QR code beside it. Type it exactly; it is case-sensitive."
	if !p.addQR(s, p.password) {
		note = "Type it exactly; it is case-sensitive."
	}
	p.add("m", p.password)
	p.add("p", note)
	p.add("h", "2. Get the kit from any one storage, then decrypt it")
	p.add("m", "openssl enc -d -aes-256-cbc -pbkdf2 -in "+kit+" | tar -xvf -")
	p.add("p", "Enter the recovery password when asked. The kit holds the whole configuration: archiver.env, secrets/ (every password and key), RECREATE.txt (how this deployment was run) and README.txt. Run the image ghcr.io/sisyphusmd/archiver with that configuration and restore with 'archiver restore'.")
	for n := 1; s.get(fmt.Sprintf("STORAGE_TARGET_%d_NAME", n)) != ""; n++ {
		p.add("rule", "")
		p.target(s, n, kit)
	}
	p.add("rule", "")
	p.add("notes", "Account recovery and 2FA codes (storage logins, email), written by hand:|3")
	p.add("p", "After printing: 'archiver envelope confirm', then delete the files. Reprint when 'archiver status' says this envelope is out of date.")
	return p
}

// credential is a storage's break-glass pair when both are set, else its backup pair.
func credential(s Source, pre, a, b string) (x, y string, breakglass bool) {
	x, y = s.Settings.Secret(pre+"BREAKGLASS_"+a), s.Settings.Secret(pre+"BREAKGLASS_"+b)
	if x == "" || y == "" {
		return s.Settings.Secret(pre + a), s.Settings.Secret(pre + b), false
	}
	return x, y, true
}

func (p *Page) access(breakglass bool) {
	if breakglass {
		p.add("p", "Credential: this storage's break-glass credential (set for reading the kit, not for backups).")
	} else {
		p.add("warn", "FULL ACCESS: this is the backup credential. Whoever holds this page can also delete the backups on this storage.")
	}
}

// target adds storage n's block; its QR code comes first, to float beside the text.
func (p *Page) target(s Source, n int, kit string) {
	pre := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
	typ := s.get(pre + "TYPE")
	role := "secondary"
	if n == 1 {
		role = "primary"
	}
	p.add("h", fmt.Sprintf("Storage %d: %s (%s, %s)", n, s.get(pre+"NAME"), typ, role))
	switch typ {
	case "local":
		path := s.get(pre + "LOCAL_PATH")
		p.add("p", "A disk attached to the backup host. No credential: whoever has the disk has the data (encrypted).")
		p.add("m", "Path in the container: "+path)
		p.add("m", "Kit: "+strings.TrimSuffix(path, "/")+"/"+kit)
	case "sftp":
		key, breakglass := s.Settings.Secret(pre+"BREAKGLASS_SSH_KEY"), true
		if key == "" {
			key, breakglass = s.sshKey(), false
		}
		if key != "" {
			p.addQR(s, key)
		}
		url, user, path := s.get(pre+"SFTP_URL"), s.get(pre+"SFTP_USER"), strings.TrimPrefix(s.get(pre+"SFTP_PATH"), "/")
		// A break-glass key may belong to its own (read-only) account.
		if bgUser := s.get(pre + "BREAKGLASS_SFTP_USER"); breakglass && bgUser != "" {
			user = bgUser
		}
		port := s.get(pre + "SFTP_PORT")
		if port == "" {
			port = "22"
		}
		p.add("m", fmt.Sprintf("Host: %s Port: %s User: %s", url, port, user))
		p.add("m", "Kit: /"+path+"/"+kit)
		p.access(breakglass)
		if key != "" {
			p.add("p", "SSH private key (save as ssh_private_key, then chmod 600):")
			for _, line := range strings.Split(key, "\n") {
				p.add("m", line)
			}
			p.add("m", fmt.Sprintf("Fetch: sftp -P %s -i ssh_private_key %s@%s:/%s/%s .", port, user, url, path, kit))
		} else {
			p.add("warn", "No SSH private key is configured, so this storage cannot be reached from this page.")
		}
	case "b2":
		id, key, breakglass := credential(s, pre, "B2_ID", "B2_KEY")
		bucket := s.get(pre + "B2_BUCKETNAME")
		p.addQR(s, "B2 bucket: "+bucket+"\nKey ID: "+id+"\nApplication key: "+key)
		p.add("m", "Backblaze B2 bucket: "+bucket)
		p.add("m", "Kit: "+kit+" (bucket root; also downloadable in the B2 web UI)")
		p.access(breakglass)
		p.add("m", "Key ID: "+id)
		p.add("m", "Application key: "+key)
	case "s3":
		id, secret, breakglass := credential(s, pre, "S3_ID", "S3_SECRET")
		bucket, endpoint, region := s.get(pre+"S3_BUCKETNAME"), s.get(pre+"S3_ENDPOINT"), s.get(pre+"S3_REGION")
		if region == "" {
			region = "none"
		}
		p.addQR(s, "S3 endpoint: "+endpoint+"\nBucket: "+bucket+"\nAccess key ID: "+id+"\nSecret key: "+secret)
		p.add("m", "S3 endpoint: "+endpoint+" Region: "+region+" Bucket: "+bucket)
		p.add("m", "Kit: "+kit+" (bucket root)")
		p.access(breakglass)
		p.add("m", "Access key ID: "+id)
		p.add("m", "Secret key: "+secret)
	}
}

// Fingerprint is what the page says (all but its date line), hashed with the recovery
// password: it identifies the page and reveals nothing about its secrets.
func (p *Page) Fingerprint() string {
	var b bytes.Buffer
	b.WriteString(p.password + "\n")
	for _, e := range p.Elements {
		if e.Kind != "sub" {
			fmt.Fprintf(&b, "%s\t%s\n", e.Kind, e.Text)
		}
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:])[:16]
}
