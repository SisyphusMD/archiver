package restore

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
)

// SnapshotExists answers whether SNAPSHOT_ID has a revision on any storage: EXISTS (0),
// NOT FOUND on every reachable storage (1), or UNDETERMINED when none is reachable (2).
// Stdout carries only the answer.
func (e *Env) SnapshotExists() int {
	id := e.getenv("SNAPSHOT_ID")
	if id == "" {
		fmt.Fprintln(e.Stderr, "ERROR: SNAPSHOT_ID environment variable is required")
		fmt.Fprintln(e.Stderr, "Usage: archiver snapshot-exists (env SNAPSHOT_ID; exit 0=exists, 1=not found, 2=unreachable, 3=a backup is running)")
		return Unreachable
	}
	if e.backupRunning() {
		fmt.Fprintln(e.Stderr, "ERROR: Archiver backup lock is held; cannot check snapshots")
		return Busy
	}
	if !e.load() {
		return NotFound
	}
	reachable := 0
	for _, t := range e.cfg.Targets {
		fmt.Fprintf(e.Stderr, "Checking '%s' (%s)...\n", t.Name, t.Type)
		revs, ok := e.probe(t, id)
		if !ok {
			continue
		}
		reachable++
		if len(revs) > 0 {
			fmt.Fprintf(e.Stderr, "  found %d revision(s)\n", len(revs))
			fmt.Fprintln(e.Stdout, "EXISTS")
			return OK
		}
		fmt.Fprintln(e.Stderr, "  no revisions")
	}
	if reachable == 0 {
		fmt.Fprintln(e.Stdout, "UNDETERMINED")
		return Unreachable
	}
	fmt.Fprintln(e.Stdout, "NOT FOUND")
	return NotFound
}

// probe lists id's revisions on t from a scratch repository; ok is false if t is
// unreachable.
func (e *Env) probe(t config.Target, id string) (revs []int, ok bool) {
	dir, err := os.MkdirTemp("", "archiver-probe-")
	if err != nil {
		fmt.Fprintln(e.Stderr, "  ", err)
		return nil, false
	}
	defer os.RemoveAll(dir)
	if err := e.connect(dir, t, id, e.Stderr); err != nil {
		fmt.Fprintln(e.Stderr, "  unreachable:", err)
		return nil, false
	}
	revs, err = e.revisions(dir, id)
	if err != nil {
		fmt.Fprintln(e.Stderr, "  list failed")
		return nil, false
	}
	return revs, true
}

// request is one non-interactive restore.
type request struct {
	id, dir  string
	revision string // a number, or "latest"
	targets  []config.Target
	opts     Options
	hook     bool
}

// Auto restores SNAPSHOT_ID into LOCAL_DIR without asking anything, from the first storage
// that has it (or the one STORAGE_TARGET names).
func (e *Env) Auto() int {
	id, dir := e.getenv("SNAPSHOT_ID"), e.getenv("LOCAL_DIR")
	for _, v := range []struct{ name, val string }{{"SNAPSHOT_ID", id}, {"LOCAL_DIR", dir}} {
		if v.val == "" {
			fmt.Fprintf(e.Stderr, "ERROR: %s environment variable is required\n", v.name)
			e.autoUsage()
			return Unreachable
		}
	}
	if e.backupRunning() {
		fmt.Fprintln(e.Stderr, "ERROR: Archiver backup lock is held; cannot run auto-restore")
		return Busy
	}
	if !e.load() {
		return NotFound
	}
	req, code := e.request(id, dir)
	if code != OK {
		return code
	}
	return e.run(req)
}

func (e *Env) autoUsage() {
	fmt.Fprint(e.Stderr, `Usage: archiver auto-restore
Required env: SNAPSHOT_ID, LOCAL_DIR
Optional env: REVISION (default 'latest'), STORAGE_TARGET (name or id),
              OVERWRITE, DELETE_EXTRA, HASH_COMPARE, IGNORE_OWNERSHIP,
              RUN_RESTORE_SERVICE (non-empty: run the service's post-restore hook
              after a successful file restore),
              RESTORE_THREADS (default matches DUPLICACY_THREADS)
Exit codes: 0=restored, 1=snapshot not found, restore failed, or the restore hook failed,
            2=unreachable or invalid env, 3=a backup is running
`)
}

// request builds a restore of id into dir from the environment's settings.
func (e *Env) request(id, dir string) (request, int) {
	req := request{id: id, dir: dir, revision: e.getenv("REVISION"), hook: e.getenv("RUN_RESTORE_SERVICE") != ""}
	if req.revision == "" {
		req.revision = "latest"
	} else if _, err := strconv.Atoi(req.revision); err != nil && req.revision != "latest" {
		fmt.Fprintf(e.Stderr, "ERROR: REVISION '%s' is not a revision number or 'latest'\n", req.revision)
		return req, Unreachable
	}
	req.opts = Options{
		Hash:        e.getenv("HASH_COMPARE") != "",
		Overwrite:   e.getenv("OVERWRITE") != "",
		Delete:      e.getenv("DELETE_EXTRA") != "",
		IgnoreOwner: e.getenv("IGNORE_OWNERSHIP") != "",
		Threads:     e.getenv("RESTORE_THREADS"),
	}
	if req.opts.Threads == "" {
		req.opts.Threads = e.cfg.Threads
	}
	if pin := e.getenv("STORAGE_TARGET"); pin != "" {
		t, ok := e.pinnedTarget(pin)
		if !ok {
			fmt.Fprintf(e.Stderr, "ERROR: STORAGE_TARGET '%s' does not match any configured storage name or id\n", pin)
			return req, Unreachable
		}
		req.targets = []config.Target{t}
	} else {
		req.targets = e.cfg.Targets
	}
	return req, OK
}

// run restores req from the first of its targets that has the revision.
func (e *Env) run(req request) int {
	dir, err := prepare(req.dir)
	if err != nil {
		fmt.Fprintf(e.Stderr, "ERROR: Failed to create '%s': %v\n", req.dir, err)
		return Unreachable
	}
	unreachable := 0
	for _, t := range req.targets {
		fmt.Fprintf(e.Stdout, "Trying '%s' (%s)...\n", t.Name, t.Type)
		if err := e.connect(dir, t, req.id, e.Stdout); err != nil {
			fmt.Fprintln(e.Stdout, "  unreachable:", err)
			unreachable++
			continue
		}
		revs, err := e.revisions(dir, req.id)
		if err != nil {
			fmt.Fprintln(e.Stdout, "  list failed")
			unreachable++
			continue
		}
		rev, ok := pick(revs, req.revision)
		switch {
		case len(revs) == 0:
			fmt.Fprintln(e.Stdout, "  no revisions")
			continue
		case !ok:
			fmt.Fprintf(e.Stdout, "  revision %s not found\n", req.revision)
			continue
		}
		fmt.Fprintf(e.Stdout, "  using revision %d\n", rev)
		fmt.Fprintf(e.Stdout, "Restoring from '%s' at revision %d...\n", t.Name, rev)
		if err := e.restore(dir, t, req.id, rev, req.opts, e.Stdout); err != nil {
			fmt.Fprintf(e.Stderr, "ERROR: Restore from '%s' failed: %v\n", t.Name, err)
			return NotFound
		}
		fmt.Fprintln(e.Stdout, "Repository restored.")
		return e.finish(dir, req, rev, t)
	}
	if unreachable == len(req.targets) {
		fmt.Fprintln(e.Stderr, "ERROR: All storage targets unreachable")
		return Unreachable
	}
	fmt.Fprintf(e.Stderr, "ERROR: Snapshot '%s' not found on any reachable storage target\n", req.id)
	return NotFound
}

// finish runs what follows a restored directory: AfterRestore, then the restore hook when
// asked for. A failed hook fails the restore with NotFound's code, never one of the codes
// that mean unreachable or busy.
func (e *Env) finish(dir string, req request, rev int, t config.Target) int {
	if e.AfterRestore != nil {
		e.AfterRestore(dir)
	}
	if !req.hook {
		return OK
	}
	code, err := e.postRestore(dir, req.id, rev, t, e.Stdin, e.Stdout)
	if err != nil {
		fmt.Fprintln(e.Stderr, "ERROR:", err)
		return NotFound
	}
	if code != 0 {
		fmt.Fprintf(e.Stderr, "ERROR: The restore hook in '%s' exited %d\n", dir, code)
		return NotFound
	}
	return OK
}

// pick resolves want ("latest" or a number) against revs, newest first.
func pick(revs []int, want string) (int, bool) {
	if len(revs) == 0 {
		return 0, false
	}
	if want == "latest" {
		return revs[0], true
	}
	n, _ := strconv.Atoi(want)
	for _, r := range revs {
		if r == n {
			return r, true
		}
	}
	return 0, false
}

// AutoAll restores every configured service directory from its own snapshot, with Auto's
// settings, and fails if any did. Files are merged into what each directory already holds
// unless OVERWRITE or DELETE_EXTRA say otherwise.
func (e *Env) AutoAll() int {
	if e.backupRunning() {
		fmt.Fprintln(e.Stderr, "ERROR: Archiver backup lock is held; cannot run auto-restore-all")
		return Busy
	}
	if !e.load() {
		return NotFound
	}
	if len(e.cfg.ServiceDirectories) == 0 {
		fmt.Fprintln(e.Stderr, "ERROR: SERVICE_DIRECTORIES is not set; auto-restore-all restores the service directories it names.")
		return NotFound
	}
	dirs, _ := config.ExpandServiceDirectories(e.cfg.ServiceDirectories)
	if len(dirs) == 0 {
		fmt.Fprintln(e.Stderr, "ERROR: No service directories resolved from SERVICE_DIRECTORIES")
		return NotFound
	}
	var restored, failed []string
	for _, dir := range dirs {
		service := filepath.Base(filepath.Clean(dir))
		id := e.Hostname + "-" + service
		fmt.Fprintf(e.Stdout, "\n=== %s: restoring '%s' -> %s ===\n", service, id, dir)
		req, code := e.request(id, dir)
		// A backup that started after the first services restored must not have its
		// repositories replaced under it.
		if code == OK && e.backupRunning() {
			fmt.Fprintln(e.Stderr, "ERROR: Archiver backup lock is held; not restoring this service")
			code = Busy
		}
		if code == OK {
			code = e.run(req)
		}
		if code == OK {
			restored = append(restored, service)
		} else {
			fmt.Fprintf(e.Stdout, "WARNING: restore failed for '%s' (exit %d)\n", service, code)
			failed = append(failed, service)
		}
	}
	fmt.Fprintln(e.Stdout, "\n=== auto-restore-all summary ===")
	fmt.Fprintf(e.Stdout, "restored: %s\n", orNone(restored))
	if len(failed) > 0 {
		fmt.Fprintf(e.Stdout, "FAILED:   %s\n", strings.Join(failed, " "))
		return NotFound
	}
	fmt.Fprintln(e.Stdout, "All services restored.")
	return OK
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, " ")
}
