// Package health implements `archiver healthcheck`, which backs the image's Docker
// HEALTHCHECK. Its exit status is the contract: non-zero only for faults a person must fix.
// A run that finished with errors stays healthy on purpose: flipping the container
// unhealthy would restart it under a liveness probe and repeat the failure notification.
package health

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

const (
	recentLines      = 100
	logFreshFor      = 48 * time.Hour
	maintenanceStale = 8 * 24 * time.Hour
	minLogSpaceMB    = 100
	backupFinished   = "Main backup script exited."
	maintFinished    = "Maintenance script exited."
)

// Env looks up an environment variable; os.Getenv in production.
type Env func(string) string

type report struct {
	w                io.Writer
	errors, warnings int
}

func (r *report) ok(format string, a ...any) { fmt.Fprintf(r.w, "OK: "+format+"\n", a...) }
func (r *report) warn(format string, a ...any) {
	r.warnings++
	fmt.Fprintf(r.w, "WARNING: "+format+"\n", a...)
}
func (r *report) fail(format string, a ...any) {
	r.errors++
	fmt.Fprintf(r.w, "ERROR: "+format+"\n", a...)
}

// Run writes the health report and returns the exit status: 1 when unhealthy, else 0.
func Run(w io.Writer, l layout.Layout, env Env, now time.Time) int {
	r := &report{w: w}

	hasKey := exists(l.RSAPrivateKey())
	if hasKey {
		r.ok("RSA private key exists")
	} else {
		r.fail("RSA private key not found")
	}
	if exists(l.SSHPrivateKey()) {
		r.ok("SSH private key exists")
	} else {
		r.warn("SSH private key not found (OK if not using SFTP)")
	}

	backupLogExists := exists(l.BackupLog())
	if !backupLogExists {
		r.warn("Log file not found (may not have run yet)")
	} else {
		// The log path is a symlink the backup repoints at each run's file, so its own
		// timestamp says when a run last started.
		if fi, err := os.Lstat(l.BackupLog()); err == nil && now.Sub(fi.ModTime()) < logFreshFor {
			r.ok("Log file is recent (modified within 48 hours)")
		} else {
			r.warn("Log file is stale (not modified in 48 hours)")
		}
		lines := lastLines(l.BackupLog(), recentLines)
		if n := countErrors(lines); n > 0 {
			// Errors without the exit marker mean the run crashed or hung mid-backup.
			if contains(lines, backupFinished) {
				r.warn("Found %d errors in recent logs, but the run finished", n)
			} else {
				r.fail("Found %d errors in recent logs without a finished run", n)
			}
		} else {
			r.ok("No errors in recent logs")
		}
	}

	if exists(l.MaintenanceLog()) {
		lines := lastLines(l.MaintenanceLog(), recentLines)
		if n := countErrors(lines); n > 0 {
			maint, held, _ := lockstate.ReadLock(l.MaintenanceLock())
			switch {
			case contains(lines, maintFinished):
				r.warn("Found %d errors in recent maintenance logs, but the run finished", n)
			case held && maint.Alive():
				// Each run starts a fresh log, so a live run's errors are not a finished
				// failure yet; one slow storage does not fail the others.
				r.warn("Found %d errors in maintenance logs; run still in progress", n)
			default:
				r.fail("Found %d errors in recent maintenance logs without a finished run", n)
			}
		} else {
			r.ok("No errors in recent maintenance logs")
		}
	}

	// A dead holder's lock heals itself on the next run, so it only warns.
	for _, lk := range []struct{ path, label string }{
		{l.BackupLock(), "Backup"}, {l.MaintenanceLock(), "Maintenance"},
	} {
		lock, held, _ := lockstate.ReadLock(lk.path)
		if !held {
			continue
		}
		if lock.Alive() {
			r.ok("%s is currently running (PID %d)", lk.label, lock.PID)
		} else {
			r.warn("Stale %s lock (will be reaped on next run): %s", lk.label, lk.path)
		}
	}

	checkOn, pruneOn := maintenanceToggles(l, env)
	if checkOn || pruneOn {
		storages, _ := lockstate.ReadMaintenance(l.MaintenanceState())
		// Secondaries kept by copy workers are no longer in bash maintenance's record; how
		// overdue their checks are is status's to show (ADR 17), not health's.
		// Only while a running daemon says its workers keep them: saved state alone outlives
		// a switch back to bash maintenance.
		workers := map[string]copier.State{}
		if reply, err := daemon.Send(l.DaemonSocket(), daemon.CmdWorkers); err == nil && reply == daemon.ReplyOK {
			workers = (&copier.Store{Path: l.CopyWorkersState()}).Load()
		}
		switch {
		case len(storages) > 0:
			for _, s := range storages {
				if _, kept := workers[s.Name]; kept {
					continue
				}
				if checkOn && now.Sub(time.Unix(s.Check, 0)) > maintenanceStale {
					r.warn("No successful check on '%s' in over 8 days", s.Name)
				}
				if pruneOn && now.Sub(time.Unix(s.Prune, 0)) > maintenanceStale {
					r.warn("No successful prune on '%s' in over 8 days", s.Name)
				}
			}
		case backupLogExists:
			r.warn("Maintenance has never completed (set MAINTENANCE_SCHEDULE or run 'archiver maintenance')")
		}
	}

	if fi, err := os.Stat(l.LogDir()); err == nil && fi.IsDir() {
		if mb, err := availableMB(l.LogDir()); err == nil {
			if mb < minLogSpaceMB {
				r.fail("Low disk space for logs (%dMB available)", mb)
			} else {
				r.ok("Sufficient disk space for logs (%dMB available)", mb)
			}
		}
	}

	fmt.Fprintf(w, "\n=== Health Check Summary ===\nErrors:   %d\nWarnings: %d\n", r.errors, r.warnings)
	switch {
	case r.errors > 0:
		fmt.Fprintln(w, "Status:   UNHEALTHY")
		return 1
	case r.warnings > 0:
		fmt.Fprintln(w, "Status:   HEALTHY (with warnings)")
	default:
		fmt.Fprintln(w, "Status:   HEALTHY")
	}
	return 0
}

// maintenanceToggles resolves CHECK_BACKUPS and PRUNE_BACKUPS (formerly ROTATE_BACKUPS)
// from the environment, defaulting to true.
func maintenanceToggles(l layout.Layout, env Env) (check, prune bool) {
	get := func(name string) string { return env(name) }
	on := func(v string) bool { return v == "" || strings.EqualFold(v, "true") }
	pruneVal := get("PRUNE_BACKUPS")
	if pruneVal == "" {
		pruneVal = get("ROTATE_BACKUPS")
	}
	return on(get("CHECK_BACKUPS")), on(pruneVal)
}

func exists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func countErrors(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, "[ERROR]") {
			n++
		}
	}
	return n
}

func contains(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// lastLines returns up to n final lines of a file, reading backwards so a large log costs
// only its tail.
func lastLines(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	const block = 64 << 10
	var buf []byte
	for off := fi.Size(); off > 0 && bytes.Count(buf, []byte{'\n'}) <= n; {
		size := int64(block)
		if off < size {
			size = off
		}
		off -= size
		chunk := make([]byte, size)
		if _, err := f.ReadAt(chunk, off); err != nil && !errors.Is(err, io.EOF) {
			return nil
		}
		buf = append(chunk, buf...)
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// availableMB is the space an unprivileged writer has left, rounded up the way `df -BM` does.
func availableMB(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	avail := int64(st.Bavail) * int64(st.Bsize)
	return (avail + (1<<20 - 1)) >> 20, nil
}
