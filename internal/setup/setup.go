// Package setup implements `archiver init` (ADR 10): it generates the keys and passwords,
// asks for the service directories, storages and notifications, and writes env-native
// materials (archiver.env plus one file per secret and key) to the setup directory. Nothing
// is taken from the container's own configuration: the answers alone are written.
package setup

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Init is one run of `archiver init`.
type Init struct {
	KeysDir  string // /opt/archiver/keys
	SetupDir string // /opt/archiver/setup
	In       io.Reader
	Out      io.Writer
	Now      func() time.Time
	// Hide turns off the terminal's echo for a secret answer and returns what restores it;
	// nil echoes everything (piped input).
	Hide func() (restore func())

	in     *bufio.Reader
	values map[string]string
	dirs   []string
}

// Run returns the exit code.
func (s *Init) Run() int {
	s.in = bufio.NewReader(s.In)
	s.values = map[string]string{}
	if err := s.run(); err != nil {
		fmt.Fprintln(s.Out, "Error:", err)
		return 1
	}
	return 0
}

func (s *Init) run() error {
	s.header("Archiver Initialization")
	if err := os.MkdirAll(s.KeysDir, 0o700); err != nil {
		return err
	}
	rsaPassphrase, err := s.rsaKeys()
	if err != nil {
		return err
	}
	if err := s.sshKeys(); err != nil {
		return err
	}

	s.header("Configuration Setup")
	storagePassword := password()
	recoveryPassword := password()
	// The kit holds the storage password, so the two must differ; a collision is theoretical.
	for recoveryPassword == storagePassword {
		recoveryPassword = password()
	}
	if err := s.serviceDirectories(); err != nil {
		return err
	}
	s.values["STORAGE_PASSWORD"] = storagePassword
	s.values["RSA_PASSPHRASE"] = rsaPassphrase
	s.values["RECOVERY_PASSWORD"] = recoveryPassword

	s.section("Storage Configuration")
	if err := s.storage(1, true); err != nil {
		return err
	}
	for n := 2; ; n++ {
		if !s.yes("\nAdd another storage target? (y/N): ") {
			break
		}
		fmt.Fprintln(s.Out)
		if err := s.storage(n, false); err != nil {
			return err
		}
	}
	if s.yes("\nSetup Pushover notifications? (y/N): ") {
		fmt.Fprintln(s.Out)
		user, err := s.ask("  Pushover user key: ")
		if err != nil {
			return fmt.Errorf("input ended before setup was complete")
		}
		token, err := s.ask("  Pushover API token: ")
		if err != nil {
			return fmt.Errorf("input ended before setup was complete")
		}
		s.values["NOTIFICATION_SERVICE"] = "Pushover"
		s.values["PUSHOVER_USER_KEY"] = user
		s.values["PUSHOVER_API_TOKEN"] = token
	}
	// Defaults (maintenance runs on MAINTENANCE_SCHEDULE or 'archiver maintenance').
	s.values["CHECK_BACKUPS"] = "true"
	s.values["PRUNE_BACKUPS"] = "true"
	s.values["PRUNE_KEEP"] = "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
	s.values["PRUNE_EXHAUSTIVE_FREQUENCY"] = "monthly"
	s.values["DUPLICACY_THREADS"] = "4"
	s.success("Configuration recorded")

	s.section("Writing Env-Native Deployment Materials")
	out := filepath.Join(s.SetupDir, "env-native")
	if err := os.MkdirAll(out, 0o700); err != nil {
		return err
	}
	keys := map[string]string{
		"rsa_private_key": filepath.Join(s.KeysDir, "private.pem"),
		"rsa_public_key":  filepath.Join(s.KeysDir, "public.pem"),
		"ssh_private_key": filepath.Join(s.KeysDir, "id_ed25519"),
		"ssh_public_key":  filepath.Join(s.KeysDir, "id_ed25519.pub"),
	}
	if err := config.NewSettings(s.dirs, s.values).WriteEnvAndSecrets(filepath.Join(out, "archiver.env"), filepath.Join(out, "secrets"), keys); err != nil {
		return err
	}
	os.Chmod(filepath.Join(out, "archiver.env"), 0o600)
	s.success("Env-native materials written to env-native/ in the mounted setup directory")
	s.credentials(recoveryPassword)
	return nil
}

func (s *Init) header(t string) {
	fmt.Fprintf(s.Out, "\n===========================================\n%s\n===========================================\n\n", t)
}

func (s *Init) section(t string) {
	fmt.Fprintf(s.Out, "\n-------------------------------------------\n%s\n-------------------------------------------\n\n", t)
}

func (s *Init) success(t string) { fmt.Fprintln(s.Out, "✓ "+t) }

// ask prints prompt and reads one line, trimmed; io.EOF once input has run out.
func (s *Init) ask(prompt string) (string, error) {
	fmt.Fprint(s.Out, prompt)
	line, err := s.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(s.Out)
		return "", io.EOF
	}
	return strings.TrimSpace(line), nil
}

// askUntil asks until clean returns a non-empty answer, printing problem otherwise.
func (s *Init) askUntil(prompt, problem string, clean func(string) string) (string, error) {
	return s.askUntilHidden(prompt, problem, clean, false)
}

func (s *Init) askUntilHidden(prompt, problem string, clean func(string) string, hidden bool) (string, error) {
	for {
		var v string
		var err error
		if hidden && s.Hide != nil {
			restore := s.Hide()
			v, err = s.ask(prompt)
			restore()
			fmt.Fprintln(s.Out)
		} else {
			v, err = s.ask(prompt)
		}
		if err != nil {
			return "", fmt.Errorf("input ended before setup was complete")
		}
		if v = clean(v); v != "" {
			return v, nil
		}
		fmt.Fprintln(s.Out, problem)
	}
}

// yes reads a y/N answer; anything but y or Y, or the end of input, is no.
func (s *Init) yes(prompt string) bool {
	v, _ := s.ask(prompt)
	return v == "y" || v == "Y"
}

const alphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// password is 32 random alphanumeric characters.
func password() string {
	b := make([]byte, 32)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphanumeric))))
		if err != nil {
			panic(err) // the kernel's random source failing is not recoverable
		}
		b[i] = alphanumeric[n.Int64()]
	}
	return string(b)
}

// setAside renames an existing file to <name>.backup.<timestamp> before it is regenerated. A
// file it cannot move (a bind-mounted key is a mount point) is an error: generating over it
// would destroy the only copy.
func (s *Init) setAside(p string) error {
	if _, err := os.Lstat(p); err != nil {
		return nil
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	aside := p + ".backup." + now().Format("20060102-150405")
	if err := os.Rename(p, aside); err != nil {
		return fmt.Errorf("cannot set the existing %s aside before generating a new one (%v); move it yourself and run init again", p, err)
	}
	fmt.Fprintln(s.Out, "ℹ Backed up existing file to: "+filepath.Base(aside))
	return nil
}

// rsaKeys generates the RSA key pair (PKCS#1 PEM, AES-256 encrypted with a generated
// passphrase, as every existing deployment's) and returns the passphrase. It reaches openssl
// on fd 3, never argv.
func (s *Init) rsaKeys() (string, error) {
	s.section("Generating RSA Encryption Keys")
	priv, pub := filepath.Join(s.KeysDir, "private.pem"), filepath.Join(s.KeysDir, "public.pem")
	for _, p := range []string{priv, pub} {
		if err := s.setAside(p); err != nil {
			return "", err
		}
	}
	pass := password()
	if err := opensslWithPass(pass, "genrsa", "-aes256", "-passout", "fd:3", "-out", priv, "-traditional", "2048"); err != nil {
		return "", fmt.Errorf("RSA key pair generation failed: %v", err)
	}
	if err := opensslWithPass(pass, "rsa", "-in", priv, "-passin", "fd:3", "-outform", "PEM", "-pubout", "-out", pub); err != nil {
		return "", fmt.Errorf("RSA public key extraction failed: %v", err)
	}
	os.Chmod(s.KeysDir, 0o700)
	os.Chmod(priv, 0o600)
	os.Chmod(pub, 0o644)
	s.success("RSA key pair generated")
	return pass, nil
}

func opensslWithPass(pass string, args ...string) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	cmd := exec.Command("openssl", args...)
	cmd.ExtraFiles = []*os.File{r}
	if err := cmd.Start(); err != nil {
		w.Close()
		return err
	}
	io.WriteString(w, pass+"\n")
	w.Close()
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (s *Init) sshKeys() error {
	s.section("Generating SSH Keys for SFTP")
	priv := filepath.Join(s.KeysDir, "id_ed25519")
	for _, p := range []string{priv, priv + ".pub"} {
		if err := s.setAside(p); err != nil {
			return err
		}
	}
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-f", priv, "-N", "", "-C", "archiver").CombinedOutput(); err != nil {
		return fmt.Errorf("SSH key pair generation failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	os.Chmod(priv, 0o600)
	os.Chmod(priv+".pub", 0o644)
	s.success("SSH key pair generated")
	return nil
}

func (s *Init) serviceDirectories() error {
	fmt.Fprintln(s.Out, "Specify directories to backup (comma-separated):")
	fmt.Fprintln(s.Out, "  - Use full paths (e.g., /srv/*, /home/user)")
	fmt.Fprintln(s.Out, "  - Use * for all subdirectories (e.g., /srv/*/ backs up each subdir separately)")
	fmt.Fprintln(s.Out)
	for len(s.dirs) == 0 {
		v, err := s.askUntil("Directories: ", "Error: At least one directory is required", func(v string) string { return v })
		if err != nil {
			return err
		}
		for _, d := range strings.Split(v, ",") {
			if d = cleanPath(d); d != "" {
				s.dirs = append(s.dirs, d)
			}
		}
		if len(s.dirs) == 0 {
			fmt.Fprintln(s.Out, "Error: At least one directory is required")
		}
	}
	return nil
}

// storage asks for storage target n.
func (s *Init) storage(n int, primary bool) error {
	if primary {
		fmt.Fprintln(s.Out, "Configure primary storage (required):")
	} else {
		fmt.Fprintf(s.Out, "Configure storage target #%d:\n", n)
	}
	fmt.Fprintln(s.Out)
	p := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
	set := func(k, v string) { s.values[p+k] = v }
	name, err := s.askUntil("  Storage name: ", "  Error: Storage name is required (letters, numbers, underscores only)", cleanName)
	if err != nil {
		return err
	}
	typ, err := s.askUntil("  Storage type ("+strings.Join(config.StorageTypes, "/")+"): ", "  Error: Must be one of "+strings.Join(config.StorageTypes, ", "), func(v string) string {
		v = strings.ToLower(strings.Join(strings.Fields(v), ""))
		for _, t := range config.StorageTypes {
			if v == t {
				return v
			}
		}
		return ""
	})
	if err != nil {
		return err
	}
	set("NAME", name)
	set("TYPE", typ)
	trim := strings.TrimSpace
	ask := func(key, prompt, problem string, clean func(string) string) error {
		v, err := s.askUntilHidden(prompt, problem, clean, key == "B2_KEY" || key == "S3_SECRET")
		if err == nil {
			set(key, v)
		}
		return err
	}
	switch typ {
	case "local":
		err = ask("LOCAL_PATH", "  Local path: ", "  Error: Path cannot be empty", cleanPath)
	case "sftp":
		if err = ask("SFTP_URL", "  SFTP host (IP or FQDN): ", "  Error: Host cannot be empty", cleanHost); err != nil {
			return err
		}
		port, _ := s.ask("  SFTP port [22]: ")
		set("SFTP_PORT", cleanPort(port))
		if err = ask("SFTP_USER", "  SFTP user: ", "  Error: User cannot be empty", trim); err != nil {
			return err
		}
		err = ask("SFTP_PATH", "  SFTP path: ", "  Error: Path cannot be empty", func(v string) string {
			return strings.Trim(cleanPath(v), "/")
		})
	case "b2":
		for _, q := range []struct{ key, prompt, problem string }{
			{"B2_BUCKETNAME", "  B2 bucket name: ", "  Error: Bucket name cannot be empty"},
			{"B2_ID", "  B2 key ID: ", "  Error: Key ID cannot be empty"},
			{"B2_KEY", "  B2 application key: ", "  Error: Application key cannot be empty"},
		} {
			if err = ask(q.key, q.prompt, q.problem, trim); err != nil {
				return err
			}
		}
	case "s3":
		if err = ask("S3_BUCKETNAME", "  S3 bucket name: ", "  Error: Bucket name cannot be empty", trim); err != nil {
			return err
		}
		if err = ask("S3_ENDPOINT", "  S3 endpoint: ", "  Error: Endpoint cannot be empty", cleanHost); err != nil {
			return err
		}
		region, _ := s.ask("  S3 region [none]: ")
		if region == "" {
			region = "none"
		}
		set("S3_REGION", region)
		if err = ask("S3_ID", "  S3 access key ID: ", "  Error: Access key ID cannot be empty", trim); err != nil {
			return err
		}
		err = ask("S3_SECRET", "  S3 secret key: ", "  Error: Secret key cannot be empty", trim)
	}
	if err != nil {
		return err
	}
	s.success("Storage target configured")
	return nil
}

var nonName = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// cleanName keeps letters, digits and underscores, hyphens becoming underscores.
func cleanName(v string) string {
	v = strings.Join(strings.Fields(v), "")
	return nonName.ReplaceAllString(strings.ReplaceAll(v, "-", "_"), "")
}

// cleanPath trims a path, refusing one with control characters.
func cleanPath(v string) string {
	v = strings.TrimSpace(v)
	if strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return ""
	}
	return v
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._:-]*[a-zA-Z0-9])?$`)

// cleanHost trims a hostname (or IP, with an optional port) and its trailing slashes.
func cleanHost(v string) string {
	v = strings.TrimRight(strings.TrimSpace(v), "/")
	if !hostname.MatchString(v) {
		return ""
	}
	return v
}

// cleanPort is a valid port, or 22.
func cleanPort(v string) string {
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 65535 {
		return strconv.Itoa(n)
	}
	return "22"
}

func (s *Init) credentials(recovery string) {
	s.header("Setup Complete!")
	fmt.Fprint(s.Out, `IMPORTANT: Save this password in your password manager NOW. Once your deployment
is running, Archiver keeps an encrypted recovery kit (every setting, secret, and
key) on EVERY storage target, refreshed automatically when anything changes. This
one password plus any one reachable storage location recovers everything, on any
machine, with no other files needed.

┌─────────────────────────────────────────────────────────────┐
│ RECOVERY PASSWORD: the single key to recover everything    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
`)
	fmt.Fprintf(s.Out, "│   %-57s │\n", recovery)
	fmt.Fprint(s.Out, `│                                                             │
└─────────────────────────────────────────────────────────────┘

`)
	if pub, err := os.ReadFile(filepath.Join(s.KeysDir, "id_ed25519.pub")); err == nil {
		fmt.Fprintf(s.Out, "SSH Public Key (for SFTP servers):\n────────────────────────────────────────────────────────────\n%s────────────────────────────────────────────────────────────\n\nCopy this key to your SFTP server's authorized_keys file.\n\n", pub)
	}
	fmt.Fprint(s.Out, `Next steps:
  1. Load env-native/archiver.env as environment variables and the
     env-native/secrets/ files as secrets mounted under /run/secrets (see the compose template),
     then DELETE env-native/ from the setup directory: it holds your secrets in plaintext.
  2. Start container: docker compose up -d

`)
}
