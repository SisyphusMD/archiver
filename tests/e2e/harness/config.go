package harness

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// AddUnmatchedService lists a service directory in the configuration without mounting
// anything there, the way a typo or a forgotten volume leaves an installation. Call it
// after Services is final and before Start.
func (d *Deployment) AddUnmatchedService(name string) {
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	d.Extra["SERVICE_DIRECTORIES"] = d.env()["SERVICE_DIRECTORIES"] + ":" + containerServiceDir(name)
}

// Secrets is every secret value the harness hands this deployment, by name.
func (d *Deployment) Secrets() map[string]string {
	s := map[string]string{
		"storage password": StoragePassword,
		"RSA passphrase":   d.Keys.Passphrase,
	}
	if d.RecoveryPassword != "" {
		s["recovery password"] = d.RecoveryPassword
	}
	return s
}

// ArgvWatch samples the argv of every process in a running deployment's container from a
// sidecar that shares its PID namespace, so it sees commands however they are started.
type ArgvWatch struct {
	dir  string
	name string
}

// WatchArgv starts watching d's processes for any of secrets and returns once the watch
// is live, so everything the caller runs afterwards is observed.
func WatchArgv(t testing.TB, d *Deployment, secrets map[string]string) *ArgvWatch {
	t.Helper()
	dir, err := os.MkdirTemp(filepath.Dir(d.secretsDir), "argvwatch-")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "argvwatch")
	build := exec.Command("go", "build", "-o", bin, "github.com/SisyphusMD/archiver/tests/e2e/harness/argvwatch")
	// Static, so it runs in whatever image the deployment uses.
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build argvwatch: %v\n%s", err, b)
	}
	var lines []string
	for _, v := range secrets {
		lines = append(lines, v)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &ArgvWatch{dir: dir, name: "archiver-e2e-argv-" + randomSuffix()}
	r := docker(t, "run", "-d", "--name", w.name, "--pid", "container:"+d.name, "--network", "none",
		"--cap-drop", "ALL", "--no-healthcheck", "-v", dir+":"+dir, "--entrypoint", bin, d.Image, dir)
	if r.Code != 0 {
		t.Fatalf("start argv watch: %s", r.Output())
	}
	t.Cleanup(func() { docker(t, "rm", "-f", w.name) })
	waitFor(t, 30*time.Second, "argv watch to start", func() bool {
		_, err := os.Stat(filepath.Join(dir, "ready"))
		return err == nil
	})
	return w
}

// Stop ends the watch. It returns how many processes started while it watched, which
// proves it saw the work, and every argv that carried a secret.
func (w *ArgvWatch) Stop(t testing.TB) (started int, leaks []string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(w.dir, "stop"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := docker(t, "wait", w.name); r.Code != 0 || strings.TrimSpace(r.Stdout) != "0" {
		t.Fatalf("argv watch failed (%s): %s", strings.TrimSpace(r.Stdout), docker(t, "logs", w.name).Output())
	}
	b, err := os.ReadFile(filepath.Join(w.dir, "result"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	started, err = strconv.Atoi(lines[0])
	if err != nil {
		t.Fatalf("argv watch result %q: %v", b, err)
	}
	return started, lines[1:]
}

// PlaintextSecrets returns "path: secret name" for every file under dirs that holds a
// secret value verbatim.
func PlaintextSecrets(t testing.TB, secrets map[string]string, dirs ...string) []string {
	t.Helper()
	var hits []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, name := range secretsIn(b, secrets) {
				hits = append(hits, p+": "+name)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return hits
}

// PlaintextSecretsInContainer is PlaintextSecrets over everything the container itself has
// written: its root filesystem and the volumes its image declares. Bind mounts, such as
// the secrets the harness provides, are not part of either.
func (d *Deployment) PlaintextSecretsInContainer(t testing.TB, secrets map[string]string) []string {
	t.Helper()
	hits := scanTar(t, secrets, "/", "export", d.name)
	r := docker(t, "image", "inspect", "--format", "{{json .Config.Volumes}}", d.Image)
	if r.Code != 0 {
		t.Fatalf("inspect %s: %s", d.Image, r.Output())
	}
	var vols map[string]struct{}
	if err := json.Unmarshal([]byte(r.Stdout), &vols); err != nil {
		t.Fatalf("volumes of %s: %v", d.Image, err)
	}
	for v := range vols {
		// docker cp names entries after the copied directory itself.
		hits = append(hits, scanTar(t, secrets, filepath.Dir(v), "cp", d.name+":"+v, "-")...)
	}
	return hits
}

func scanTar(t testing.TB, secrets map[string]string, prefix string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("docker", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var hits []string
	tr := tar.NewReader(bufio.NewReader(out))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("docker %s: %v", strings.Join(args, " "), err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range secretsIn(b, secrets) {
			hits = append(hits, filepath.Join(prefix, h.Name)+": "+name)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return hits
}

func secretsIn(b []byte, secrets map[string]string) []string {
	var names []string
	for name, v := range secrets {
		if bytes.Contains(b, []byte(v)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func waitFor(t testing.TB, timeout time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Bundle is a 0.11 encrypted configuration bundle: openssl enc -aes-256-cbc -pbkdf2 over a
// tar of config.sh and keys/.
type Bundle struct {
	Path     string
	Password string
}

// NewBundle writes d's configuration, secrets, and keys into a bundle under dir, byte for
// byte the way 0.11 `bundle export` does. legacy writes the shape older releases left
// behind instead: an init-generated config.sh (array SERVICE_DIRECTORIES, ROTATE_BACKUPS)
// encrypted with `openssl -k`. The bundle only carries configuration; mounts stay d's.
func NewBundle(t testing.TB, d *Deployment, dir string, legacy bool) Bundle {
	t.Helper()
	stage, err := os.MkdirTemp(dir, "bundle-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "keys"), 0o700); err != nil {
		t.Fatal(err)
	}
	for src, dst := range map[string]string{d.Keys.PrivatePath: "private.pem", d.Keys.PublicPath: "public.pem"} {
		if err := os.WriteFile(filepath.Join(stage, "keys", dst), mustRead(t, src), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var cfg strings.Builder
	if legacy {
		cfg.WriteString("# Directories to backup\nSERVICE_DIRECTORIES=(\n")
		for _, svc := range d.serviceNames() {
			fmt.Fprintf(&cfg, "  %q\n", containerServiceDir(svc)+"/")
		}
		cfg.WriteString(")\n\n")
	} else {
		fmt.Fprintf(&cfg, "SERVICE_DIRECTORIES=%s\n", d.env()["SERVICE_DIRECTORIES"])
	}
	fmt.Fprintf(&cfg, "STORAGE_PASSWORD=%q\nRSA_PASSPHRASE=%q\n", StoragePassword, d.Keys.Passphrase)
	if d.RecoveryPassword != "" {
		fmt.Fprintf(&cfg, "RECOVERY_PASSWORD=%q\n", d.RecoveryPassword)
	}
	env := d.env()
	keys := make([]string, 0, len(env))
	for k := range env {
		if k != "SERVICE_DIRECTORIES" && (!legacy || k != "ROTATE_BACKUPS") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&cfg, "%s=%q\n", k, env[k])
	}
	if legacy {
		cfg.WriteString("ROTATE_BACKUPS=\"true\"\nPRUNE_KEEP=\"-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1\"\nDUPLICACY_THREADS=\"4\"\n")
	}
	if err := os.WriteFile(filepath.Join(stage, "config.sh"), []byte(cfg.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	b := Bundle{Path: filepath.Join(stage, "bundle.tar.enc"), Password: "e2e-bundle-password"}
	pass := `-pass fd:3 3<<<"$2"`
	if legacy {
		pass = `-k "$2"`
	}
	script := `set -e; cd "$1"; tar -cf bundle.tar keys config.sh
openssl enc -aes-256-cbc -pbkdf2 -salt -in bundle.tar -out bundle.tar.enc ` + pass + `
rm -rf bundle.tar keys config.sh`
	if out, err := exec.Command("bash", "-c", script, "bash", stage, b.Password).CombinedOutput(); err != nil {
		t.Fatalf("write bundle: %v\n%s", err, out)
	}
	return b
}

// StartWithBundle runs d configured by nothing but b: the bundle and its password, with
// d's hostname, service directories, storages, and restore root mounted as usual.
func (d *Deployment) StartWithBundle(t testing.TB, b Bundle) {
	t.Helper()
	secrets, err := os.MkdirTemp(filepath.Dir(b.Path), "secrets-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "bundle_password"), []byte(b.Password), 0o600); err != nil {
		t.Fatal(err)
	}
	d.Mount(b.Path, "/opt/archiver/bundle/bundle.tar.enc")
	d.startRaw(t, nil, secrets)
}

// Migrated is an installation configured solely by what the migration produced.
type Migrated struct {
	*Deployment
	env        map[string]string
	secretsDir string
}

// Start runs the migrated configuration with the original deployment's mounts.
func (m *Migrated) Start(t testing.TB) {
	t.Helper()
	m.startRaw(t, m.env, m.secretsDir)
}

// MigrateBundle has image convert b into env-native configuration for installation d,
// through the 0.11 `migrate` command run in a bundle-configured container. The result
// carries none of the harness's own env or secrets, so it works only if the migration did.
func MigrateBundle(t testing.TB, image string, b Bundle, d *Deployment) *Migrated {
	t.Helper()
	out, err := os.MkdirTemp(filepath.Dir(b.Path), "migrated-")
	if err != nil {
		t.Fatal(err)
	}
	src := *d
	src.Image = image
	src.mounts = nil
	src.Mount(out, "/migrate-out")
	src.StartWithBundle(t, b)
	if r := src.Archiver(t, nil, "migrate", "/migrate-out"); r.Code != 0 {
		t.Fatalf("migrate by %s exited %d:\n%s", image, r.Code, r.Output())
	}
	src.Stop(t)

	env := map[string]string{}
	for _, line := range strings.Split(string(mustRead(t, filepath.Join(out, "archiver.env"))), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(k, "#") {
			env[k] = v
		}
	}
	dst := *d
	dst.Image = image
	dst.mounts = nil
	return &Migrated{Deployment: &dst, env: env, secretsDir: filepath.Join(out, "secrets")}
}

// startRaw is Start with the configuration supplied verbatim instead of derived from d.
func (d *Deployment) startRaw(t testing.TB, env map[string]string, secretsDir string) {
	t.Helper()
	d.name = "archiver-e2e-" + randomSuffix()
	d.secretsDir = secretsDir
	args := []string{"run", "-d", "--name", d.name, "--hostname", d.Hostname,
		"--cap-drop", "ALL", "--cap-add", "DAC_OVERRIDE", "--cap-add", "CHOWN", "--cap-add", "FOWNER",
		"-v", secretsDir + ":/run/secrets:ro"}
	for _, svc := range d.serviceNames() {
		args = append(args, "-v", d.Services[svc]+":"+containerServiceDir(svc))
	}
	for i, s := range d.Storages {
		args = append(args, "-v", s.Dir+":"+containerStorageDir(i))
	}
	if d.RestoreRoot != "" {
		args = append(args, "-v", d.RestoreRoot+":"+containerRestoreRoot)
	}
	for _, m := range d.mounts {
		args = append(args, "-v", m[0]+":"+m[1])
	}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, d.Image)
	if r := docker(t, args...); r.Code != 0 {
		t.Fatalf("start %s: %s", d.Image, r.Output())
	}
	name := d.name
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("logs of %s (%s):\n%s", name, d.Image, docker(t, "logs", name).Output())
		}
		docker(t, "rm", "-f", name)
	})
	d.waitStarted(t)
}

// waitStarted returns once the entrypoint has put the configuration in place. docker run
// returns while a bundle is still being decrypted, and a command exec'd in that window
// finds no configuration and fails. 0.11 surface: `healthcheck` passes only once the
// configuration and keys exist.
func (d *Deployment) waitStarted(t testing.TB) {
	t.Helper()
	waitFor(t, 60*time.Second, d.Image+" to finish starting", func() bool {
		return d.Archiver(t, nil, "healthcheck").Code == 0
	})
}
