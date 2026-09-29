package harness

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Schedules in the 0.11 syntax (supercronic, which takes an optional seconds field). The
// syntax is incidental (ADR 1); only "invalid fails start, valid runs" is contract.
const (
	InvalidSchedule  = "not a cron line"
	FrequentSchedule = "*/5 * * * * * *"
	DailySchedule    = "0 3 * * *"
)

var capFlags = []string{"--cap-drop", "ALL", "--cap-add", "DAC_OVERRIDE", "--cap-add", "CHOWN", "--cap-add", "FOWNER"}

// ScheduleBackups and ScheduleMaintenance make the container run that pipeline on spec
// instead of waiting for manual commands. Call before Start.
func (d *Deployment) ScheduleBackups(spec string)     { d.setExtra("BACKUP_SCHEDULE", spec) }
func (d *Deployment) ScheduleMaintenance(spec string) { d.setExtra("MAINTENANCE_SCHEDULE", spec) }

// InheritHostname hands the container a HOSTNAME environment variable that differs from
// its kernel hostname, the way a Kubernetes Job pins its snapshot IDs across pod names.
func (d *Deployment) InheritHostname(h string) { d.setExtra("HOSTNAME", h) }

func (d *Deployment) setExtra(k, v string) {
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	d.Extra[k] = v
}

// Healthcheck runs the healthcheck the image itself declares, inside the running
// container, so the test judges whatever the image tells Docker to judge it by.
func (d *Deployment) Healthcheck(t testing.TB) Result {
	t.Helper()
	// exec into a stopped container fails too, which would read as "unhealthy".
	if !d.Running(t) {
		t.Fatalf("healthcheck of %s: container is not running", d.Image)
	}
	r := docker(t, "inspect", "-f", "{{json .Config.Healthcheck}}", d.name)
	if r.Code != 0 {
		t.Fatalf("inspect %s: %s", d.name, r.Output())
	}
	var hc struct{ Test []string }
	if err := json.Unmarshal([]byte(r.Stdout), &hc); err != nil {
		t.Fatalf("parse healthcheck of %s: %v: %s", d.Image, err, r.Stdout)
	}
	switch {
	case len(hc.Test) >= 2 && hc.Test[0] == "CMD-SHELL":
		return docker(t, "exec", d.name, "sh", "-c", hc.Test[1])
	case len(hc.Test) >= 2 && hc.Test[0] == "CMD":
		return docker(t, append([]string{"exec", d.name}, hc.Test[1:]...)...)
	}
	t.Fatalf("%s declares no healthcheck: %v", d.Image, hc.Test)
	return Result{}
}

// StartWithLowLogSpace starts the deployment with its log directory on a volume too small
// to keep logging, a condition the healthcheck must call unhealthy.
func (d *Deployment) StartWithLowLogSpace(t testing.TB) {
	t.Helper()
	d.startWith(t, []string{"--tmpfs", "/opt/archiver/logs:size=16m"})
}

// StartWithoutRSAKey starts the deployment normally, except that it was never given the
// RSA private key.
func (d *Deployment) StartWithoutRSAKey(t testing.TB) {
	t.Helper()
	d.writeSecrets(t)
	if err := os.Remove(filepath.Join(d.secretsDir, "rsa_private_key")); err != nil {
		t.Fatal(err)
	}
	d.startWith(t, nil)
}

// WaitExit polls until the container stops or timeout passes, and reports its exit code.
func (d *Deployment) WaitExit(t testing.TB, timeout time.Duration) (code int, exited bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		r := docker(t, "inspect", "-f", "{{.State.Running}} {{.State.ExitCode}}", d.name)
		if r.Code != 0 {
			t.Fatalf("inspect %s: %s", d.name, r.Output())
		}
		running, c, _ := strings.Cut(strings.TrimSpace(r.Stdout), " ")
		if running == "false" {
			n, err := strconv.Atoi(c)
			if err != nil {
				t.Fatalf("exit code of %s: %q", d.name, c)
			}
			return n, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Running reports whether the container is still up.
func (d *Deployment) Running(t testing.TB) bool {
	t.Helper()
	r := docker(t, "inspect", "-f", "{{.State.Running}}", d.name)
	return r.Code == 0 && strings.TrimSpace(r.Stdout) == "true"
}

// RecoverFromKit stands up a fresh deployment from nothing but an unpacked recovery kit
// (see OpenKit), following the kit's own recreation notes: its settings as environment,
// its secrets/ mounted as the secrets, the physical storages mounted where the kit's
// settings expect each storage name, and an empty directory at every service path. The
// directory layout and names read here are the 0.11 kit format, incidental (ADR 1).
func RecoverFromKit(t testing.TB, image, kitDir, hostname string, storages map[string]string, restoreRoot string) *Deployment {
	t.Helper()
	env := readEnvFile(t, filepath.Join(kitDir, "archiver.env"))
	d := &Deployment{Image: image, Hostname: hostname, RestoreRoot: restoreRoot, secretsDir: filepath.Join(kitDir, "secrets")}

	for n := 1; env["STORAGE_TARGET_"+strconv.Itoa(n)+"_NAME"] != ""; n++ {
		p := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
		name := env[p+"NAME"]
		host, ok := storages[name]
		if !ok {
			t.Fatalf("the kit names storage %q, which the recovery was not given", name)
		}
		d.mounts = append(d.mounts, [2]string{host, env[p+"LOCAL_PATH"]})
	}
	for _, svc := range strings.Split(env["SERVICE_DIRECTORIES"], ":") {
		if svc == "" {
			continue
		}
		empty := filepath.Join(filepath.Dir(kitDir), "empty-"+randomSuffix())
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		d.mounts = append(d.mounts, [2]string{empty, svc})
	}

	args := append([]string{"--hostname", hostname}, capFlags...)
	args = append(args, "-v", d.secretsDir+":/run/secrets:ro", "-v", restoreRoot+":"+containerRestoreRoot)
	for _, m := range d.mounts {
		args = append(args, "-v", m[0]+":"+m[1])
	}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	d.run(t, append(args, image))
	return d
}

func readEnvFile(t testing.TB, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	env := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		if k, v, ok := strings.Cut(s.Text(), "="); ok && k != "" && !strings.HasPrefix(k, "#") {
			env[k] = v
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return env
}

// BackupJob runs one backup as a one-shot container that exits with the backup's result,
// the way a Kubernetes Job does (0.11 `run backup`). Unlike Backup, the backup runs under
// the image's own start-up, so it sees whatever environment that start-up leaves it.
func (d *Deployment) BackupJob(t testing.TB) Result {
	t.Helper()
	name := "archiver-e2e-" + randomSuffix()
	args := append([]string{"run", "--rm", "--name", name}, d.runArgs(t, nil)...)
	return docker(t, append(args, d.Image, "run", "backup")...)
}

// startWith is Start with extra docker run flags.
func (d *Deployment) startWith(t testing.TB, flags []string) {
	t.Helper()
	d.run(t, append(d.runArgs(t, flags), d.Image))
}

// runArgs is every docker run argument Start would pass before the image, plus flags.
func (d *Deployment) runArgs(t testing.TB, flags []string) []string {
	t.Helper()
	if d.secretsDir == "" {
		d.writeSecrets(t)
	}
	args := append([]string{"--hostname", d.Hostname}, capFlags...)
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
	return append(args, flags...)
}

// run starts the container detached under a fresh name and removes it after the test.
func (d *Deployment) run(t testing.TB, args []string) {
	t.Helper()
	d.name = "archiver-e2e-" + randomSuffix()
	if r := docker(t, append([]string{"run", "-d", "--name", d.name}, args...)...); r.Code != 0 {
		t.Fatalf("start %s: %s", d.Image, r.Output())
	}
	name := d.name
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("logs of %s (%s):\n%s", name, d.Image, docker(t, "logs", name).Output())
		}
		docker(t, "rm", "-f", name)
	})
}
