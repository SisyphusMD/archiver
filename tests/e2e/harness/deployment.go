package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Storage is one storage target. The first one in a Deployment is the primary.
type Storage struct {
	Name string
	Dir  string // host path of a local storage
}

// MinimalCaps is every capability archiver may need: DAC_OVERRIDE to read and write files
// of other owners, CHOWN and FOWNER for a restore to recreate owners, modes, and times.
var MinimalCaps = []string{"DAC_OVERRIDE", "CHOWN", "FOWNER"}

// Deployment is one archiver installation: an image, a hostname (which fixes the snapshot
// IDs), service directories, storages, and secrets. Two Deployments that share hostname,
// services, storages, and keys are the same installation run by different images, which
// is exactly what an upgrade is.
type Deployment struct {
	Image    string
	Hostname string
	Services map[string]string // service name -> host path
	Storages []Storage
	Keys     Keys
	// RecoveryPassword enables the recovery kit when set.
	RecoveryPassword string
	// Extra holds additional environment for the container.
	Extra map[string]string
	// RestoreRoot is a host directory mounted for restores; see Restore.
	RestoreRoot string
	// Caps are the only capabilities the container gets on top of --cap-drop ALL; nil
	// means MinimalCaps, the set archiver documents.
	Caps []string

	name       string
	secretsDir string
	mounts     [][2]string
}

// SnapshotID is the ID archiver gives a service of this deployment.
func (d *Deployment) SnapshotID(service string) string {
	return d.Hostname + "-" + service
}

// Start runs the container in manual mode (no schedule) and removes it after the test.
func (d *Deployment) Start(t testing.TB) {
	t.Helper()
	d.name = "archiver-e2e-" + randomSuffix()
	d.writeSecrets(t)

	args := []string{"run", "-d", "--name", d.name, "--hostname", d.Hostname, "--cap-drop", "ALL"}
	caps := d.Caps
	if caps == nil {
		caps = MinimalCaps
	}
	for _, c := range caps {
		args = append(args, "--cap-add", c)
	}
	args = append(args, "-v", d.secretsDir+":/run/secrets:ro")
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
	for k, v := range d.env() {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, d.Image)

	if r := docker(t, args...); r.Code != 0 {
		t.Fatalf("start %s: %s", d.Image, r.Output())
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("logs of %s (%s):\n%s", d.name, d.Image, docker(t, "logs", d.name).Output())
		}
		docker(t, "rm", "-f", d.name)
	})
}

// Stop removes the container early, so another image can take over the installation.
func (d *Deployment) Stop(t testing.TB) {
	t.Helper()
	docker(t, "rm", "-f", d.name)
}

// Archiver runs `archiver <args>` inside the running container.
func (d *Deployment) Archiver(t testing.TB, env map[string]string, args ...string) Result {
	t.Helper()
	full := []string{"exec"}
	for k, v := range env {
		full = append(full, "-e", k+"="+v)
	}
	full = append(full, d.name, "archiver")
	return docker(t, append(full, args...)...)
}

// Mount adds a bind mount for commands such as restore that need a target directory. It
// must be called before Start.
func (d *Deployment) Mount(hostDir, containerDir string) {
	d.mounts = append(d.mounts, [2]string{hostDir, containerDir})
}

func (d *Deployment) serviceNames() []string {
	names := make([]string, 0, len(d.Services))
	for n := range d.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func containerServiceDir(service string) string { return "/srv/" + service }
func containerStorageDir(i int) string          { return fmt.Sprintf("/storage/%d", i+1) }

// env is the 0.11 configuration surface. Its names are incidental (ADR 1): when v1 renames
// them, this becomes one of two surfaces and the upgrade test drives the migration.
func (d *Deployment) env() map[string]string {
	env := map[string]string{}
	var dirs string
	for i, svc := range d.serviceNames() {
		if i > 0 {
			dirs += ":"
		}
		dirs += containerServiceDir(svc)
	}
	env["SERVICE_DIRECTORIES"] = dirs
	for i, s := range d.Storages {
		n := i + 1
		env[fmt.Sprintf("STORAGE_TARGET_%d_NAME", n)] = s.Name
		env[fmt.Sprintf("STORAGE_TARGET_%d_TYPE", n)] = "local"
		env[fmt.Sprintf("STORAGE_TARGET_%d_LOCAL_PATH", n)] = containerStorageDir(i)
	}
	env["ROTATE_BACKUPS"] = "false"
	for k, v := range d.Extra {
		env[k] = v
	}
	return env
}

func (d *Deployment) writeSecrets(t testing.TB) {
	t.Helper()
	dir, err := os.MkdirTemp(filepath.Dir(d.Keys.PrivatePath), "secrets-")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"storage_password": []byte(StoragePassword),
		"rsa_passphrase":   []byte(d.Keys.Passphrase),
		"rsa_private_key":  mustRead(t, d.Keys.PrivatePath),
		"rsa_public_key":   mustRead(t, d.Keys.PublicPath),
	}
	if d.RecoveryPassword != "" {
		files["recovery_password"] = []byte(d.RecoveryPassword)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d.secretsDir = dir
}

func mustRead(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
