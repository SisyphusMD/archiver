package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// A container with a bad schedule has nothing to wait for before refusing, so this is
// generous; the point is that it exits instead of idling forever.
const promptExit = 60 * time.Second

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLifecycleRecoveryKit proves the kit is refreshed by the backup after a config change
// and that it alone recovers the installation: the second backup adds a storage, and a
// deployment rebuilt from nothing but that storage-held kit and its password (the original
// keys, secrets, and data deleted) must restore both revisions from both storages. A kit
// left over from the first backup knows nothing of the added storage and cannot pass.
func TestLifecycleRecoveryKit(t *testing.T) {
	_, current := images(t)
	work := harness.WorkDir(t)
	svc := filepath.Join(work, "svc", "app")
	primary := filepath.Join(work, "storage-primary")
	offsite := filepath.Join(work, "storage-offsite")
	restores := filepath.Join(work, "restores")
	keysDir := filepath.Join(work, "keys")
	mkdirs(t, svc, primary, offsite, restores, keysDir)

	keys := harness.NewKeys(t, current, keysDir)
	const host, password = "e2e-host", "e2e-recovery-password"
	install := func(storages ...harness.Storage) *harness.Deployment {
		return &harness.Deployment{
			Image:            current,
			Hostname:         host,
			Services:         map[string]string{"app": svc},
			Storages:         storages,
			Keys:             keys,
			RecoveryPassword: password,
		}
	}

	writeFixtures(t, svc, 1)
	v1 := harness.Snapshot(t, svc)
	first := install(harness.Storage{Name: "primary", Dir: primary})
	first.Start(t)
	if r := first.Backup(t); r.Code != 0 {
		t.Fatalf("first backup exited %d:\n%s", r.Code, r.Output())
	}
	first.Stop(t)

	writeFixtures(t, svc, 2)
	v2 := harness.Snapshot(t, svc)
	second := install(harness.Storage{Name: "primary", Dir: primary}, harness.Storage{Name: "offsite", Dir: offsite})
	second.Start(t)
	if r := second.Backup(t); r.Code != 0 {
		t.Fatalf("backup after adding a storage exited %d:\n%s", r.Code, r.Output())
	}
	second.Stop(t)

	// The disaster: everything but the storages is gone.
	for _, d := range []string{keysDir, svc} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}
	unpacked := filepath.Join(work, "rescue")
	if err := os.CopyFS(unpacked, os.DirFS(harness.OpenKit(t, harness.KitPath(primary, host), password))); err != nil {
		t.Fatal(err)
	}

	rescued := harness.RecoverFromKit(t, current, unpacked, host,
		map[string]string{"primary": primary, "offsite": offsite}, restores)
	for _, c := range []struct {
		storage string
		rev     int
		want    harness.Tree
	}{
		{"primary", 1, v1}, {"primary", 2, v2}, {"offsite", 1, v1}, {"offsite", 2, v2},
	} {
		got := harness.Snapshot(t, rescued.Restore(t, "app", c.rev, c.storage))
		if d := c.want.Diff(got); len(d) > 0 {
			t.Errorf("restore of rev %d from %s by the kit-rebuilt deployment differs: %v", c.rev, c.storage, d)
		}
	}
}

// A deployment that refuses to start is as visibly unhealthy as one whose healthcheck
// fails, so an unhealthy case gets this long to exit before its healthcheck is run.
const settle = 15 * time.Second

// TestLifecycleHealthcheck judges the image's own healthcheck by its exit code: zero for a
// deployment that has backed up, non-zero for one without its RSA key or without room to
// log. An unhealthy deployment may instead refuse to run at all, with a non-zero exit.
func TestLifecycleHealthcheck(t *testing.T) {
	_, current := images(t)
	setup := func(t *testing.T) *harness.Deployment {
		work := harness.WorkDir(t)
		svc := filepath.Join(work, "svc", "app")
		primary := filepath.Join(work, "storage-primary")
		keysDir := filepath.Join(work, "keys")
		mkdirs(t, svc, primary, keysDir)
		writeFixtures(t, svc, 1)
		return &harness.Deployment{
			Image:    current,
			Hostname: "e2e-host",
			Services: map[string]string{"app": svc},
			Storages: []harness.Storage{{Name: "primary", Dir: primary}},
			Keys:     harness.NewKeys(t, current, keysDir),
		}
	}

	t.Run("healthy after a backup", func(t *testing.T) {
		d := setup(t)
		d.Start(t)
		if r := d.Backup(t); r.Code != 0 {
			t.Fatalf("backup exited %d:\n%s", r.Code, r.Output())
		}
		if r := d.Healthcheck(t); r.Code != 0 {
			t.Errorf("healthcheck of a deployment that just backed up exited %d:\n%s", r.Code, r.Output())
		}
	})
	for _, c := range []struct {
		name  string
		start func(*harness.Deployment, testing.TB)
	}{
		{"unhealthy without the RSA key", (*harness.Deployment).StartWithoutRSAKey},
		{"unhealthy without room to log", (*harness.Deployment).StartWithLowLogSpace},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := setup(t)
			c.start(d, t)
			if code, exited := d.WaitExit(t, settle); exited {
				if code == 0 {
					t.Errorf("container exited 0 instead of reporting unhealthy")
				}
				return
			}
			if r := d.Healthcheck(t); r.Code == 0 {
				t.Errorf("healthcheck exited 0:\n%s", r.Output())
			}
		})
	}
}

// TestLifecycleInvalidSchedule proves a malformed backup or maintenance schedule stops the
// container promptly with a non-zero exit, rather than leaving it up and never backing up,
// while a valid schedule keeps it up and does back up.
func TestLifecycleInvalidSchedule(t *testing.T) {
	_, current := images(t)
	setup := func(t *testing.T) (*harness.Deployment, string) {
		work := harness.WorkDir(t)
		svc := filepath.Join(work, "svc", "app")
		primary := filepath.Join(work, "storage-primary")
		keysDir := filepath.Join(work, "keys")
		mkdirs(t, svc, primary, keysDir)
		writeFixtures(t, svc, 1)
		return &harness.Deployment{
			Image:    current,
			Hostname: "e2e-host",
			Services: map[string]string{"app": svc},
			Storages: []harness.Storage{{Name: "primary", Dir: primary}},
			Keys:     harness.NewKeys(t, current, keysDir),
		}, primary
	}

	for _, c := range []struct {
		name                string
		backup, maintenance string
	}{
		{"backup schedule", harness.InvalidSchedule, ""},
		{"maintenance schedule", harness.DailySchedule, harness.InvalidSchedule},
	} {
		t.Run("invalid "+c.name, func(t *testing.T) {
			d, _ := setup(t)
			d.ScheduleBackups(c.backup)
			if c.maintenance != "" {
				d.ScheduleMaintenance(c.maintenance)
			}
			d.Start(t)
			code, exited := d.WaitExit(t, promptExit)
			switch {
			case !exited:
				t.Fatalf("container still running %s after start with an invalid %s", promptExit, c.name)
			case code == 0:
				t.Fatalf("container exited 0 with an invalid %s", c.name)
			}
		})
	}

	t.Run("valid schedule backs up", func(t *testing.T) {
		d, primary := setup(t)
		d.ScheduleBackups(harness.FrequentSchedule)
		d.Start(t)
		deadline := time.Now().Add(90 * time.Second)
		for len(harness.Revisions(t, primary, d.SnapshotID("app"))) == 0 {
			if !d.Running(t) {
				t.Fatal("container with a valid schedule stopped")
			}
			if time.Now().After(deadline) {
				t.Fatal("no scheduled backup reached storage within 90s")
			}
			time.Sleep(time.Second)
		}
	})
}

// TestLifecycleInheritedHostname is a never-break check: an inherited HOSTNAME variable,
// not the kernel hostname, names the snapshot ID. Kubernetes Jobs get a new pod name each
// run and rely on this to keep extending one snapshot ID. Both ways a backup runs are
// checked: in a running container, and as a one-shot Job under the image's start-up.
func TestLifecycleInheritedHostname(t *testing.T) {
	_, current := images(t)
	for _, c := range []struct {
		name   string
		backup func(*harness.Deployment, *testing.T) harness.Result
	}{
		{"manual backup", func(d *harness.Deployment, t *testing.T) harness.Result {
			d.Start(t)
			return d.Backup(t)
		}},
		{"one-shot job", func(d *harness.Deployment, t *testing.T) harness.Result { return d.BackupJob(t) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			work := harness.WorkDir(t)
			svc := filepath.Join(work, "svc", "app")
			primary := filepath.Join(work, "storage-primary")
			keysDir := filepath.Join(work, "keys")
			mkdirs(t, svc, primary, keysDir)
			writeFixtures(t, svc, 1)

			d := &harness.Deployment{
				Image:    current,
				Hostname: "kernel-host",
				Services: map[string]string{"app": svc},
				Storages: []harness.Storage{{Name: "primary", Dir: primary}},
				Keys:     harness.NewKeys(t, current, keysDir),
				// The kit is named after the host too, and must follow the same name, or a
				// Job leaves a new kit on storage every run.
				RecoveryPassword: "e2e-recovery-password",
			}
			d.InheritHostname("inherited-host")
			if r := c.backup(d, t); r.Code != 0 {
				t.Fatalf("backup exited %d:\n%s", r.Code, r.Output())
			}
			if got := harness.Revisions(t, primary, "inherited-host-app"); !slices.Equal(got, []int{1}) {
				t.Errorf("revisions of inherited-host-app = %v, want [1]", got)
			}
			if got := harness.Revisions(t, primary, "kernel-host-app"); len(got) > 0 {
				t.Errorf("backup went to the kernel hostname's ID kernel-host-app: %v", got)
			}
			if _, err := os.Stat(harness.KitPath(primary, "inherited-host")); err != nil {
				t.Errorf("no recovery kit named after the inherited hostname: %v", err)
			}
			if _, err := os.Stat(harness.KitPath(primary, "kernel-host")); err == nil {
				t.Errorf("recovery kit named after the kernel hostname")
			}
		})
	}
}
