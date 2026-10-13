package kit

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/envelope"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

const signInNote = "Fetched with Archiver's token; the provider sign-in step is left to a person."

// Drill rehearses fetching and opening each offsite kit using the envelope's access.
// It shares the restore drill lock so stop, pause and resume act on either kind.
func (r *Run) Drill(storage string) int {
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	r.ctx = ctx
	defer func() { r.ctx = nil }()
	n := notify.FromConfig(&config.Config{}, r.Hostname, nil)
	n.Incidents = r.Layout.Incidents()
	fail := func(message string) int {
		r.error(message)
		if err := lockstate.WriteKitDrills(r.Layout.KitDrillState()+".run", map[string]lockstate.KitDrillResult{
			"": {At: time.Now().Unix(), Message: message},
		}); err != nil {
			r.error("Kit drill: cannot save the failed run result: " + err.Error())
		}
		n.Raise("kit-drill", notify.Failure, "Kit Drill Failed", message)
		return Failed
	}
	lock, _, err := runlock.Acquire(r.Layout.DrillLock(), r.Layout.DrillStopFlag(), "kit-drill", "starting")
	if err != nil {
		r.error("Kit drill: could not take the drill lock: " + err.Error())
		return Failed
	}
	defer lock.Release()
	if err := r.cleanDrillScratch(); err != nil {
		return fail("Kit drill: cannot remove interrupted private scratch: " + err.Error())
	}
	cfg, _, err := config.Load(r.Source, r.Environ)
	if cfg != nil {
		n = notify.FromConfig(cfg, r.Hostname, nil)
		n.Incidents = r.Layout.Incidents()
	}
	if err == nil {
		err = cfg.ValidateStorage(r.Source.SecretsDir)
	}
	if err != nil {
		return fail(err.Error())
	}
	r.cfg = cfg
	limit, err := cfg.KitDrillTimeout()
	if err != nil {
		return fail(err.Error())
	}
	if cfg.RecoveryPassword == "" {
		return fail("Kit drill: RECOVERY_PASSWORD is not configured.")
	}
	var targets []config.Target
	for _, t := range cfg.Targets {
		if t.Type != "local" && (storage == "" || storage == t.Name || storage == t.StorageName()) {
			targets = append(targets, t)
		}
	}
	if storage != "" && len(targets) == 0 {
		return fail("Kit drill: '" + storage + "' is not a configured offsite storage.")
	}
	// Paused work does not consume the run's time budget.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		remaining, last := limit, time.Now()
		paused := lock.State().Paused()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				nowPaused := lock.State().Paused()
				if !paused && !nowPaused {
					remaining -= now.Sub(last)
				}
				paused = nowPaused
				last = now
				if remaining <= 0 || lock.StopRequested() {
					cancel()
					return
				}
			}
		}
	}()
	gate := func() error {
		for lock.State().Paused() && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-time.After(100 * time.Millisecond):
			}
		}
		return ctx.Err()
	}
	record, err := resume.Begin(r.Layout.RunRecord("kit-drill"), resume.Record{})
	if err != nil {
		return fail("Kit drill: cannot record the run.")
	}
	defer func() {
		pending, _ := resume.Read(r.Layout.RunRecord("kit-drill"))
		if len(pending.Services) == 0 && (!lock.StopRequested() || !resume.ShuttingDown(r.Layout.ShutdownFlag())) {
			record.End()
		}
	}()
	results, err := lockstate.ReadKitDrillStorages(r.Layout.KitDrillState())
	if err != nil {
		return fail("Kit drill: cannot read the results.")
	}
	if err := lockstate.WriteKitDrills(r.Layout.KitDrillState(), results); err != nil {
		return fail("Kit drill: cannot save the results.")
	}
	if err := os.Remove(r.Layout.KitDrillState() + ".run"); err != nil && !os.IsNotExist(err) {
		return fail("Kit drill: cannot clear the failed run result.")
	}
	n.Clear("kit-drill", "Kit Drill Ready", "Kit drill initialization succeeds again.")
	code := OK
	for _, t := range targets {
		started := time.Now()
		result := lockstate.KitDrillResult{At: started.Unix(), LastSuccess: results[t.StorageName()].LastSuccess}
		_ = lock.SetStage("kit-drill:"+t.StorageName(), "fetch")
		err := gate()
		if err == nil {
			result.SignInRequired, err = r.drillOne(ctx, gate, t, record)
		}
		result.Seconds = time.Since(started).Seconds()
		result.OK = err == nil
		if ctx.Err() != nil {
			err = errors.New("stopped or KIT_DRILL_TIMEOUT exceeded")
			result.OK = false
		}
		if err != nil {
			result.Message = err.Error()
		} else {
			result.LastSuccess = time.Now().Unix()
			if result.SignInRequired {
				result.Message = signInNote
			}
		}
		results[t.StorageName()] = result
		if err := lockstate.WriteKitDrills(r.Layout.KitDrillState(), results); err != nil {
			code = fail("Kit drill: cannot save the results.")
		}
		key := "kit-drill:" + t.StorageName()
		if !result.OK {
			code = Failed
			msg := fmt.Sprintf("Kit drill on storage '%s' failed: %s", t.Name, result.Message)
			r.error(msg)
			n.Raise(key, notify.Failure, "Kit Drill Failed", msg)
		} else {
			r.info(fmt.Sprintf("Kit drill on storage '%s' passed. %s", t.Name, result.Message))
			n.Clear(key, "Kit Drill Passing", fmt.Sprintf("Kit drill on storage '%s' passes again.", t.Name))
		}
	}
	if code == OK {
		_ = lock.Record("completed")
	} else {
		_ = lock.Record("failed")
	}
	return code
}

// The layout scopes cleanup to this installation, including a kill between mkdir and recording it.
func (r *Run) drillScratchPrefix() string {
	path, _ := filepath.Abs(r.Layout.RunRecord("kit-drill"))
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("archiver-kit-drill-%x-", sum[:16])
}

func (r *Run) cleanDrillScratch() error {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return err
	}
	name := regexp.MustCompile("^" + regexp.QuoteMeta(r.drillScratchPrefix()) + "[0-9]+$")
	for _, entry := range entries {
		if !name.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(os.TempDir(), entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || info.Mode().Perm() != 0o700 || int(stat.Uid) != os.Geteuid() {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func (r *Run) drillOne(ctx context.Context, gate func() error, t config.Target, record *resume.Run) (rotates bool, err error) {
	work, err := os.MkdirTemp("", r.drillScratchPrefix())
	if err != nil {
		return false, errors.New("cannot create private scratch directory")
	}
	defer func() {
		if os.RemoveAll(work) != nil {
			err = errors.New("cannot delete private scratch directory")
			return
		}
		record.Forget(work)
	}()
	if err := record.Set(work, resume.Pending); err != nil {
		return false, errors.New("cannot record private scratch directory")
	}
	settings, err := config.Snapshot(r.Source, r.Environ)
	if err != nil {
		return false, errors.New("cannot read current configuration")
	}
	source := envelope.Source{Settings: settings, Hostname: r.Hostname, SSHKeyFile: r.Layout.SSHPrivateKey()}
	values, rotates := source.Access(t.N, t.Type)
	fetched := filepath.Join(work, r.KitName())
	if err := gate(); err != nil {
		return rotates, err
	}
	if t.Type == "sftp" || t.Type == "sftpc" {
		err = r.fetchSSH(ctx, source, t, work, fetched)
	} else {
		var remote map[string]string
		var dir string
		if rotates {
			remote, dir, err = r.remote(ctx, t)
		} else {
			remote, dir, err = envelope.FetchSettings(t.Type, values, r.obscure)
		}
		if err == nil {
			cmd := drillCommand(ctx, r.rclone(), "--config", os.DevNull, "copyto", "kit:"+strings.TrimSuffix(dir, "/")+"/"+r.KitName(), fetched)
			// No inherited remote or provider environment can broaden the printed credential.
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + work}
			for k, v := range remote {
				cmd.Env = append(cmd.Env, "RCLONE_CONFIG_KIT_"+k+"="+v)
			}
			err = cmd.Run()
		}
	}
	if err != nil {
		return rotates, errors.New("cannot fetch the kit with the envelope credential (rotated credential, missing kit or unreachable storage)")
	}
	if err := gate(); err != nil {
		return rotates, err
	}
	plain := filepath.Join(work, "payload.tar")
	if err := decryptDrill(ctx, fetched, plain, r.cfg.RecoveryPassword); err != nil {
		return rotates, errors.New("envelope mismatch: the kit does not decrypt with the current recovery password")
	}
	expected := filepath.Join(work, "expected")
	if err := os.Mkdir(expected, 0o700); err != nil {
		return rotates, err
	}
	// Snapshot again: fetching OneDrive may have refreshed its token.
	settings, err = config.Snapshot(r.Source, r.Environ)
	if err != nil {
		return rotates, errors.New("cannot read current secrets")
	}
	reported := r.Log.Reportable()
	if _, err := r.payload(expected, settings); err != nil || r.Log.Reportable() > reported {
		return rotates, errors.New("cannot build the local kit payload")
	}
	if err := gate(); err != nil {
		return rotates, err
	}
	return rotates, compareDrill(ctx, plain, expected)
}

func (r *Run) fetchSSH(ctx context.Context, source envelope.Source, t config.Target, work, out string) error {
	key, user, _ := source.SSHAccess(t.N)
	if key == "" {
		return errors.New("no envelope SSH key")
	}
	keyFile := filepath.Join(work, "ssh_private_key")
	if err := os.WriteFile(keyFile, []byte(key+"\n"), 0o600); err != nil {
		return err
	}
	known, _, err := r.knownHosts(ctx, t)
	if err != nil {
		return err
	}
	port := t.Get("SFTP_PORT")
	if port == "" {
		port = "22"
	}
	cmd := drillCommand(ctx, "sftp", "-q", "-P", port, "-i", keyFile, "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+known, "-F", os.DevNull, "-b", "-", "--", user+"@"+t.Get("SFTP_URL"))
	remote := "/" + strings.Trim(t.Get("SFTP_PATH"), "/") + "/" + r.KitName()
	cmd.Stdin = strings.NewReader("get " + sftpQuote(remote) + " " + sftpQuote(out) + "\n")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + work}
	return cmd.Run()
}

func sftpQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", "", "\r", "").Replace(s) + `"`
}

func decryptDrill(ctx context.Context, in, out, password string) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	defer pw.Close()
	if err := os.WriteFile(out, nil, 0o600); err != nil {
		return err
	}
	cmd := drillCommand(ctx, "openssl", "enc", "-d", "-aes-256-cbc", "-pbkdf2", "-pass", "fd:3", "-in", in, "-out", out)
	cmd.ExtraFiles = []*os.File{pr}
	if err := cmd.Start(); err != nil {
		return err
	}
	pr.Close()
	_, err = io.WriteString(pw, password+"\n")
	pw.Close()
	wait := cmd.Wait()
	if err != nil {
		return err
	}
	return wait
}

// expectedEntries derives all files from the same payload builder used for kit placement.
func expectedEntries(dir string) (map[string]string, error) {
	return expectedEntriesContext(context.Background(), dir)
}

func expectedEntriesContext(ctx context.Context, dir string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != dir {
			name, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(name)] = ""
		}
		if d.Type().IsRegular() {
			name, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			sum, err := hashFileContext(ctx, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(name)] = sum
		}
		return nil
	})
	return files, err
}

// Never extract an untrusted tar: only paths and streamed hashes enter the comparison.
func compareDrill(ctx context.Context, archive, expected string) error {
	files, err := expectedEntriesContext(ctx, expected)
	if err != nil {
		return errors.New("cannot hash local kit")
	}
	f, err := os.Open(archive)
	if err != nil {
		return errors.New("cannot read decrypted kit")
	}
	defer f.Close()
	tr := tar.NewReader(f)
	seen := map[string]bool{}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("invalid recovery kit archive")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name {
			return errors.New("unsafe recovery kit entry")
		}
		if seen[name] {
			return errors.New("duplicate recovery kit entry")
		}
		seen[name] = true
		if h.Typeflag == tar.TypeDir {
			want, ok := files[name]
			if !ok || want != "" {
				return errors.New("kit differs from the local kit (unexpected directory)")
			}
			delete(files, name)
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return errors.New("unsafe recovery kit entry")
		}
		want, ok := files[name]
		if !ok {
			return errors.New("kit differs from the local kit (unexpected entry)")
		}
		sum := sha256.New()
		if _, err := io.Copy(sum, contextReader{ctx, tr}); err != nil {
			return errors.New("cannot hash recovery kit")
		}
		if hex.EncodeToString(sum.Sum(nil)) != want {
			if strings.HasPrefix(name, "secrets/") {
				return errors.New("kit differs from the local kit: envelope mismatch (secrets are no longer current)")
			}
			return errors.New("kit differs from the local kit")
		}
		if strings.HasPrefix(name, "extra/") {
			info, err := os.Stat(filepath.Join(expected, name))
			if err != nil || info.Mode().Perm() != fs.FileMode(h.Mode&0o777) {
				return errors.New("kit differs from the local kit (extra file mode)")
			}
		}
		delete(files, name)
	}
	if len(files) > 0 {
		return errors.New("kit differs from the local kit: missing expected entries")
	}
	return nil
}

func drillCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	return cmd
}
