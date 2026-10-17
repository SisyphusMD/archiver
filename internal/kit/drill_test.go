package kit

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/metrics"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
	"github.com/SisyphusMD/archiver/internal/status"
)

func drillFixture(t *testing.T, typ string) (*fixture, string) {
	t.Helper()
	f := newFixture(t, 1)
	os.MkdirAll(f.run.Layout.Lock, 0o700)
	f.vars["STORAGE_TARGET_1_TYPE"] = typ
	delete(f.vars, "STORAGE_TARGET_1_LOCAL_PATH")
	if typ == "minio" {
		f.vars["STORAGE_TARGET_1_S3_ENDPOINT"] = "offsite.invalid"
		f.vars["STORAGE_TARGET_1_S3_BUCKETNAME"] = "bucket"
		for name, value := range map[string]string{"s3_id": "backup-id", "s3_secret": "backup-secret", "breakglass_s3_id": "glass-id", "breakglass_s3_secret": "glass-secret"} {
			os.WriteFile(filepath.Join(f.run.Source.SecretsDir, "storage_target_1_"+name), []byte(value), 0o600)
		}
	}
	archive := filepath.Join(f.root, "remote.enc")
	f.run.Rclone = filepath.Join(f.root, "fetch")
	os.WriteFile(f.run.Rclone, []byte(`#!/bin/sh
root=$(dirname "$0")
[ ! -e "$root/fail" ] || exit 1
if [ "$RCLONE_CONFIG_KIT_TYPE" = s3 ]; then
 [ "$RCLONE_CONFIG_KIT_ACCESS_KEY_ID" = glass-id ] || exit 2
 [ "$RCLONE_CONFIG_KIT_SECRET_ACCESS_KEY" = glass-secret ] || exit 2
 [ -z "$AWS_ACCESS_KEY_ID" ] || exit 3
fi
if [ "$RCLONE_CONFIG_KIT_TYPE" = onedrive ]; then
 case "$RCLONE_CONFIG_KIT_TOKEN" in *own-token*) ;; *) exit 4 ;; esac
fi
for arg in "$@"; do out="$arg"; done
cp "$root/remote.enc" "$out"
`), 0o700)
	return f, archive
}

func buildDrillKit(t *testing.T, f *fixture, out string) {
	t.Helper()
	f.run.Environ = f.environ()
	cfg, _, err := config.Load(f.run.Source, f.run.Environ)
	if err != nil {
		t.Fatal(err)
	}
	f.run.cfg = cfg
	settings, err := config.Snapshot(f.run.Source, f.run.Environ)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if _, err := f.run.payload(work, settings); err != nil {
		t.Fatal(err)
	}
	if err := f.run.encrypt(work, out); err != nil {
		t.Fatal(err)
	}
}

func TestKitDrillPassStaleSecretsAndIncidentRecovery(t *testing.T) {
	f, archive := drillFixture(t, "minio")
	t.Setenv("AWS_ACCESS_KEY_ID", "must-not-inherit")
	buildDrillKit(t, f, archive)
	if code := f.run.Drill(""); code != OK {
		t.Fatalf("pass: %s", f.out.String())
	}
	first, _ := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
	if !first["store1"].OK || first["store1"].LastSuccess == 0 {
		t.Fatal("success not recorded")
	}
	os.WriteFile(filepath.Join(f.run.Source.SecretsDir, "storage_password"), []byte("new-private-secret"), 0o600)
	if code := f.run.Drill("store1"); code != Failed {
		t.Fatal("stale kit passed")
	}
	if !strings.Contains(f.out.String(), "differs from the local kit") || strings.Contains(f.out.String(), "new-private-secret") || strings.Contains(f.out.String(), "glass-secret") {
		t.Fatal("stale failure unclear or secrets leaked")
	}
	results, _ := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
	if results["store1"].OK || results["store1"].LastSuccess != first["store1"].LastSuccess {
		t.Fatal("failed run lost last success")
	}
	if len(notify.OpenIncidents(f.run.Layout.Incidents())) != 1 {
		t.Fatal("incident not raised")
	}
	buildDrillKit(t, f, archive)
	if f.run.Drill("") != OK {
		t.Fatal(f.out.String())
	}
	if len(notify.OpenIncidents(f.run.Layout.Incidents())) != 0 {
		t.Fatal("pass did not clear incident")
	}
	os.WriteFile(filepath.Join(f.root, "fail"), nil, 0o600)
	if f.run.Drill("") != Failed || !strings.Contains(f.out.String(), "storage 'store1'") {
		t.Fatal("fetch failure must name storage")
	}
}

func TestKitDrillOneDriveOwnTokenAndNote(t *testing.T) {
	for _, typ := range []string{"one", "odb"} {
		t.Run(typ, func(t *testing.T) {
			f, archive := drillFixture(t, typ)
			pre := "STORAGE_TARGET_1_" + strings.ToUpper(typ) + "_"
			f.vars[pre+"CLIENT_ID"] = "app"
			f.vars[pre+"PATH"] = "backups"
			os.WriteFile(filepath.Join(f.run.Source.SecretsDir, strings.ToLower(pre)+"client_secret"), []byte("client-secret"), 0o600)
			os.WriteFile(filepath.Join(f.run.Source.SecretsDir, strings.ToLower(pre)+"token"), []byte(`{"access_token":"own-token","expiry":"2099-01-01T00:00:00Z"}`), 0o600)
			oldTokenDir := config.TokenDir
			config.TokenDir = filepath.Join(f.root, "tokens")
			defer func() { config.TokenDir = oldTokenDir }()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer own-token" {
					t.Error("fetch did not use own token")
				}
				io.WriteString(w, `{"id":"drive","driveType":"personal"}`)
			}))
			defer srv.Close()
			oldGraph := graphURL
			graphURL = srv.URL
			defer func() { graphURL = oldGraph }()
			buildDrillKit(t, f, archive)
			if f.run.Drill("") != OK {
				t.Fatal(f.out.String())
			}
			results, _ := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
			if !results["store1"].SignInRequired || results["store1"].Message != signInNote {
				t.Fatal("missing human sign-in note")
			}
		})
	}
}

func TestKitDrillTimeout(t *testing.T) {
	f, archive := drillFixture(t, "minio")
	f.vars["KIT_DRILL_TIMEOUT"] = "1s"
	buildDrillKit(t, f, archive)
	os.WriteFile(f.run.Rclone, []byte("#!/bin/sh\nsleep 30\n"), 0o700)
	start := time.Now()
	if f.run.Drill("") != Failed || time.Since(start) > 5*time.Second {
		t.Fatal("timeout did not end fetch and its children")
	}
	if !strings.Contains(f.out.String(), "KIT_DRILL_TIMEOUT") {
		t.Fatal("timeout not named")
	}
}

func TestExpectedEntriesAndUnsafeArchive(t *testing.T) {
	f, archive := drillFixture(t, "minio")
	extra := filepath.Join(f.root, "runbook")
	os.WriteFile(extra, []byte("restore instructions"), 0o600)
	f.vars["RECOVERY_KIT_EXTRA_PATHS"] = extra
	os.MkdirAll(f.run.deploymentDir(), 0o700)
	os.WriteFile(filepath.Join(f.run.deploymentDir(), "compose.yaml"), []byte("services: {}"), 0o600)
	buildDrillKit(t, f, archive)
	expected := t.TempDir()
	settings, _ := config.Snapshot(f.run.Source, f.run.Environ)
	f.run.payload(expected, settings)
	entries, err := expectedEntries(expected)
	if err != nil || entries["secrets/storage_password"] == "" || entries["RECREATE.txt"] == "" || entries["extra/runbook"] == "" || entries["deployment/compose.yaml"] == "" {
		t.Fatal("entries not derived from payload")
	}
	plain := filepath.Join(t.TempDir(), "kit.tar")
	if err := decryptDrill(context.Background(), (*exec.Cmd).Start, archive, plain, f.run.cfg.RecoveryPassword); err != nil {
		t.Fatal(err)
	}
	if err := compareDrill(context.Background(), plain, expected); err != nil {
		t.Fatal(err)
	}
	for _, header := range []*tar.Header{
		{Name: "../escaped", Mode: 0o600, Typeflag: tar.TypeReg},
		{Name: "secrets/key", Linkname: "/etc/shadow", Typeflag: tar.TypeSymlink},
		{Name: "archiver.env", Typeflag: tar.TypeReg},
	} {
		out, _ := os.Create(plain)
		tw := tar.NewWriter(out)
		tw.WriteHeader(header)
		tw.Close()
		out.Close()
		if err := compareDrill(context.Background(), plain, expected); err == nil {
			t.Fatal("unsafe or incomplete kit passed")
		}
	}
}

func TestKitDrillPasswordOffArgv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "openssl"), []byte(`#!/bin/sh
case "$*" in *sentinel-recovery-password*) exit 1 ;; esac
[ "$(cat <&3)" = sentinel-recovery-password ] || exit 2
`), 0o700)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	if err := decryptDrill(context.Background(), (*exec.Cmd).Start, filepath.Join(dir, "input"), filepath.Join(dir, "output"), "sentinel-recovery-password"); err != nil {
		t.Fatal("password must arrive only on fd 3:", err)
	}
}

func TestKitDrillExcludesLocal(t *testing.T) {
	f := newFixture(t, 1)
	os.MkdirAll(f.run.Layout.Lock, 0o700)
	f.run.Environ = f.environ()
	if f.run.Drill("") != OK {
		t.Fatal(f.out.String())
	}
	results, _ := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
	if len(results) != 0 {
		t.Fatal("local-type storage was drilled")
	}
	if f.run.Drill("store1") != Failed {
		t.Fatal("named local storage must be rejected")
	}
}

func TestKitDrillPauseAndStop(t *testing.T) {
	f, archive := drillFixture(t, "minio")
	f.vars["KIT_DRILL_TIMEOUT"] = "1s"
	buildDrillKit(t, f, archive)
	os.WriteFile(f.run.Rclone, []byte("#!/bin/sh\nsleep 30\n"), 0o700)
	done := make(chan int, 1)
	go func() { done <- f.run.Drill("") }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, held, _ := lockstate.ReadLock(f.run.Layout.DrillLock()); held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drill did not take lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := runlock.Append(f.run.Layout.DrillLock(), "paused", time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("pause consumed timeout")
	case <-time.After(1500 * time.Millisecond):
	}
	os.WriteFile(f.run.Layout.DrillStopFlag(), nil, 0o600)
	select {
	case code := <-done:
		if code != Failed {
			t.Fatal("stopped drill passed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop failed to cancel a paused drill")
	}
	if _, ok := resume.Read(f.run.Layout.RunRecord("kit-drill")); ok {
		t.Fatal("manual stop left an interrupted record")
	}
}

func TestKitDrillCannotPassWhenLocalPayloadReportsMissingExtra(t *testing.T) {
	f, archive := drillFixture(t, "minio")
	f.vars["RECOVERY_KIT_EXTRA_PATHS"] = filepath.Join(f.root, "missing-runbook")
	buildDrillKit(t, f, archive)
	if f.run.Drill("") != Failed {
		t.Fatal("local payload errors must not clear a kit drill incident")
	}
}

func TestKitDrillInitializationFailureAndRecovery(t *testing.T) {
	for _, problem := range []string{"password", "config", "timeout", "corrupt-results", "unwritable-results"} {
		t.Run(problem, func(t *testing.T) {
			f, archive := drillFixture(t, "minio")
			buildDrillKit(t, f, archive)
			if f.run.Drill("") != OK {
				t.Fatal(f.out.String())
			}
			before, _ := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
			switch problem {
			case "password":
				os.Remove(filepath.Join(f.run.Source.SecretsDir, "recovery_password"))
			case "config":
				f.vars["STORAGE_TARGET_1_TYPE"] = "unknown"
			case "timeout":
				f.vars["KIT_DRILL_TIMEOUT"] = "broken"
			case "corrupt-results":
				os.WriteFile(f.run.Layout.KitDrillState(), []byte("invalid JSON"), 0o600)
			case "unwritable-results":
				os.Remove(f.run.Layout.KitDrillState())
				os.Mkdir(f.run.Layout.KitDrillState(), 0o700)
			}
			f.run.Environ = f.environ()
			if f.run.Drill("") != Failed {
				t.Fatal("initialization failure passed")
			}
			if _, ok := notify.OpenIncidents(f.run.Layout.Incidents())["kit-drill"]; !ok {
				t.Fatal("run-level incident missing")
			}
			results, err := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
			if err != nil {
				t.Fatal(err)
			}
			if result, ok := results[""]; !ok || result.OK || result.At == 0 || result.Message == "" {
				t.Fatal("failed run not recorded", results)
			}
			for _, result := range results {
				if result.OK {
					t.Fatal("old pass still visible")
				}
			}
			var report strings.Builder
			if err := status.Write(&report, f.run.Layout, f.run.Source.Getenv, time.Now()); err != nil || !strings.Contains(report.String(), "FAILED") {
				t.Fatal("status did not show failed initialization", err, report.String())
			}
			output := metrics.Render(status.Take(f.run.Layout, f.run.Source.Getenv, time.Now()))
			if !strings.Contains(output, `archiver_kit_drill_last_ok{storage=""} 0`) {
				t.Fatal("metrics did not show failed initialization", output)
			}
			os.WriteFile(filepath.Join(f.run.Source.SecretsDir, "recovery_password"), []byte("recovery-pw-123"), 0o600)
			f.vars["STORAGE_TARGET_1_TYPE"] = "minio"
			delete(f.vars, "KIT_DRILL_TIMEOUT")
			if problem == "unwritable-results" {
				os.Remove(f.run.Layout.KitDrillState())
			}
			if err := lockstate.WriteKitDrills(f.run.Layout.KitDrillState(), before); err != nil {
				t.Fatal(err)
			}
			f.run.Environ = f.environ()
			if f.run.Drill("") != OK {
				t.Fatal(f.out.String())
			}
			if _, ok := notify.OpenIncidents(f.run.Layout.Incidents())["kit-drill"]; ok {
				t.Fatal("initialization recovery did not clear incident")
			}
			after, err := lockstate.ReadKitDrills(f.run.Layout.KitDrillState())
			if err != nil || !after["store1"].OK {
				t.Fatal("recovery result missing", err)
			}
			if _, ok := after[""]; ok {
				t.Fatal("failed run survived recovery")
			}
		})
	}
}

func TestKitDrillSignalHelper(t *testing.T) {
	root := os.Getenv("KIT_DRILL_SIGNAL_ROOT")
	if root == "" {
		return
	}
	f, _ := drillFixture(t, "minio")
	f.run.Layout.Root = root
	f.run.Layout.Lock = filepath.Join(root, "lock")
	f.run.Environ = f.environ()
	script := `#!/bin/sh
root="$1"
# HOME is the private scratch of this fetch.
printf '%s' "$HOME" > "$root/scratch"
printf 'decrypted kit and secrets' > "$HOME/payload.tar"
sleep 30 &
echo $! > "$root/child"
touch "$root/ready"
wait
`
	// rclone receives flags, so the shared test path is embedded as a quoted literal.
	script = strings.Replace(script, `root="$1"`, "root='"+root+"'", 1)
	if err := os.WriteFile(f.run.Rclone, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	code := OK
	if os.Getenv("KIT_DRILL_SIGNAL_DAEMON") == "true" {
		daemon.Run(context.Background(), daemon.RealClock, io.Discard, nil, []string{"kit-drill"}, func(daemon.Job) int {
			code = f.run.Drill("")
			return code
		})
	} else {
		code = f.run.Drill("")
	}
	if code != Failed {
		t.Fatal("interrupted drill passed")
	}
}

func waitDrillFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			return b
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("drill did not create", path)
	return nil
}

func drillChildAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// Linux keeps a killed orphan as a zombie until its parent reaps it.
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && strings.Contains(string(b), ") Z ") {
		return false
	}
	return true
}

func TestKitDrillSignalsAndKilledScratch(t *testing.T) {
	for _, daemonRun := range []bool{false, true} {
		for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL} {
			t.Run(fmt.Sprintf("daemon=%t/%s", daemonRun, sig), func(t *testing.T) {
				root := t.TempDir()
				os.Mkdir(filepath.Join(root, "logs"), 0o700)
				os.Mkdir(filepath.Join(root, "lock"), 0o700)
				tmp := filepath.Join(root, "tmp")
				os.Mkdir(tmp, 0o700)
				start := func() *exec.Cmd {
					cmd := exec.Command(os.Args[0], "-test.run=^TestKitDrillSignalHelper$")
					cmd.Env = append(os.Environ(), "KIT_DRILL_SIGNAL_ROOT="+root, "TMPDIR="+tmp, "KIT_DRILL_SIGNAL_DAEMON="+strconv.FormatBool(daemonRun))
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if cmd.ProcessState == nil {
							_ = cmd.Process.Kill()
							_ = cmd.Wait()
						}
					})
					return cmd
				}
				cmd := start()
				waitDrillFile(t, filepath.Join(root, "ready"))
				results, err := lockstate.ReadKitDrills(filepath.Join(root, "logs", ".kit-drill-state.json"))
				if err != nil || results["store1"].At == 0 || results["store1"].OK || !strings.Contains(results["store1"].Message, "interrupted") {
					t.Fatalf("pending result: %+v, %v", results, err)
				}
				scratch := string(waitDrillFile(t, filepath.Join(root, "scratch")))
				child, err := strconv.Atoi(strings.TrimSpace(string(waitDrillFile(t, filepath.Join(root, "child")))))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				select {
				case err := <-done:
					if sig != syscall.SIGKILL && err != nil {
						t.Fatal("signal bypassed drill cleanup", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("signal did not stop drill")
				}
				if sig == syscall.SIGKILL {
					if _, err := os.Stat(filepath.Join(scratch, "payload.tar")); err != nil {
						t.Fatal("kill fixture has no sensitive scratch", err)
					}
					// Neighbors must survive, even if the resume record names them.
					neighbors := []string{scratch + "-unrelated", filepath.Join(tmp, "archiver-kit-drill-12345"), scratch + "9"}
					for _, path := range neighbors {
						os.Mkdir(path, 0o700)
					}
					os.Chmod(neighbors[2], 0o755)
					link := scratch + "8"
					os.Symlink(neighbors[0], link)
					neighbors = append(neighbors, link)
					// A kill can also happen before the scratch is journaled.
					services := map[string]string{}
					for _, path := range neighbors {
						services[path] = resume.Pending
					}
					if _, err := resume.Begin(filepath.Join(root, "logs", ".run-kit-drill.json"), resume.Record{Services: services}); err != nil {
						t.Fatal(err)
					}
					os.Remove(filepath.Join(root, "ready"))
					next := start()
					waitDrillFile(t, filepath.Join(root, "ready"))
					if _, err := os.Lstat(scratch); !os.IsNotExist(err) {
						t.Fatal("SIGKILL scratch survived restart", err)
					}
					for _, path := range neighbors {
						if _, err := os.Lstat(path); err != nil {
							t.Fatal("cleanup removed unrelated path", path, err)
						}
					}
					if err := next.Process.Signal(syscall.SIGTERM); err != nil {
						t.Fatal(err)
					}
					if err := next.Wait(); err != nil {
						t.Fatal(err)
					}
					return
				}
				if _, err := os.Stat(scratch); !os.IsNotExist(err) {
					t.Fatal("signal left decrypted scratch", err)
				}
				deadline := time.Now().Add(time.Second)
				for drillChildAlive(child) && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if drillChildAlive(child) {
					t.Fatal("signal left subprocess alive")
				}
			})
		}
	}
}
