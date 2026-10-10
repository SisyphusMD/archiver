package restore

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/SisyphusMD/archiver/internal/config"
)

// errEOF ends an interactive restore whose input ran out before it was answered.
var errEOF = fmt.Errorf("input ended before the restore was set up")

// Interactive asks for a storage, snapshot ID, destination and revision, restores it, and
// offers to run the restore hook. It returns 1 if anything failed, the hook included.
func (e *Env) Interactive() int {
	// The restore replaces the destination's repository, which a running backup may be using.
	if e.backupRunning() {
		fmt.Fprintln(e.Stderr, "ERROR: A backup is running. Restore once it has finished (or stop it with 'archiver stop backup').")
		return 1
	}
	if !e.load() {
		return 1
	}
	defer e.release()
	in := bufio.NewReader(e.Stdin)
	code, err := e.interactive(in)
	if err != nil {
		fmt.Fprintln(e.Stderr, "ERROR:", err)
		return 1
	}
	return code
}

func (e *Env) interactive(in *bufio.Reader) (int, error) {
	out := e.Stdout
	ask := func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		line, err := in.ReadString('\n')
		if err == io.EOF && line == "" {
			fmt.Fprintln(out)
			return "", errEOF
		}
		return strings.TrimSpace(line), nil
	}
	required := func(prompt, what string) (string, error) {
		for {
			v, err := ask(prompt)
			if err != nil || v != "" {
				return v, err
			}
			fmt.Fprintf(out, "Error: %s is required.\n", what)
		}
	}

	fmt.Fprintln(out, "Which of the following storage targets would you like to restore from?")
	for _, t := range e.cfg.Targets {
		fmt.Fprintf(out, " - %d) %s\n", t.N, t.Name)
	}
	var choice int
	for {
		v, err := ask("Enter the number of your choice: ")
		if err != nil {
			return 1, err
		}
		if n, err := strconv.Atoi(v); err == nil {
			if _, ok := e.pinnedTarget(v); ok {
				choice = n
				break
			}
		}
		fmt.Fprintf(out, "Invalid choice. Please enter a valid number between 1 and %d.\n", len(e.cfg.Targets))
	}
	t, _ := e.pinnedTarget(strconv.Itoa(choice))

	fmt.Fprintln(out)
	id, err := required("Snapshot ID to restore (required; the storage's 'snapshots' directory lists them): ", "Snapshot ID")
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "Chosen Snapshot ID: %s\n\n", id)
	var dir string
	if live := e.serviceDirOf(id); live != "" {
		// Restoring over a running service can leave it reading half-restored files; a copy
		// beside it can be checked first, and leaves the service and its repository alone.
		other := filepath.Join(os.TempDir(), "archiver-restore", id)
		fmt.Fprintf(out, "%s is the backup of %s. Restoring there replaces files a running service may be using: stop it first,\nor restore into another directory to look before copying anything back.\n", id, live)
		if dir, err = ask(fmt.Sprintf("Local directory path to restore to (Enter for %s): ", other)); err != nil {
			return 1, err
		}
		if dir == "" {
			dir = other
		}
	} else if dir, err = required("Local directory path to restore to (required): ", "Local directory path"); err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "Chosen local directory path: %s\n", dir)
	// Created only once the restore is confirmed: the preview reads it, if it exists.
	if dir, err = filepath.Abs(dir); err != nil {
		return 1, err
	}
	// Revisions and the preview come from a scratch repository: the destination's own is
	// replaced only once the restore is confirmed, so cancelling leaves it as it was.
	scratch, err := os.MkdirTemp("", "archiver-restore-")
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(scratch)
	if err := attach(scratch, t, id); err != nil {
		return 1, err
	}
	revs, err := e.revisions(scratch, id)
	if err != nil {
		return 1, fmt.Errorf("cannot list the revisions of '%s' on '%s'", id, t.Name)
	}
	if len(revs) == 0 {
		return 1, fmt.Errorf("'%s' has no revisions on '%s'", id, t.Name)
	}
	fmt.Fprintf(out, "\nRevisions of %s on %s, newest first:", id, t.Name)
	for _, r := range revs {
		fmt.Fprintf(out, " %d", r)
	}
	fmt.Fprintln(out)
	var rev int
	for {
		v, err := required(fmt.Sprintf("Choose a revision number to restore (newest is %d): ", revs[0]), "Revision number")
		if err != nil {
			return 1, err
		}
		if r, ok := pick(revs, v); ok {
			rev = r
			break
		}
		fmt.Fprintf(out, "Revision %s is not one of the revisions above.\n", v)
	}
	fmt.Fprintf(out, "Chosen revision: %d\n", rev)

	o := Options{IgnoreOwner: e.getenv("IGNORE_OWNERSHIP") != "", Threads: e.cfg.Threads}
	fmt.Fprint(out, "\nCustomize restore options (advanced)? (y/N): ")
	if yes(in) {
		for _, q := range []struct {
			prompt string
			flag   *bool
		}{
			{"Detect file differences by hash (slower but more thorough)? (y/N): ", &o.Hash},
			{"Overwrite existing files in the restore directory? (y/N): ", &o.Overwrite},
			{"Delete files not in the snapshot? (WARNING: Removes extra files) (y/N): ", &o.Delete},
			{"Ignore original file ownership (useful when restoring to different machine)? (y/N): ", &o.IgnoreOwner},
			{"Continue even if errors occur? (y/N): ", &o.Persist},
		} {
			fmt.Fprint(out, q.prompt)
			if yes(in) {
				*q.flag = true
			}
		}
		// An answer that never came ends the restore: going on would restore everything with
		// whatever overwrite and delete were chosen.
		v, err := ask(fmt.Sprintf("Download thread count (Enter keeps %s): ", o.Threads))
		if err != nil {
			return 1, err
		}
		if v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				o.Threads = v
			} else {
				fmt.Fprintf(out, "'%s' is not a thread count; keeping %s.\n", v, o.Threads)
			}
		}
		if v, err = ask("Restore only some paths in the snapshot (comma-separated; Enter for everything): "); err != nil {
			return 1, err
		}
		if v != "" {
			if o.Paths = Paths(v); len(o.Paths) == 0 {
				return 1, fmt.Errorf("'%s' names no path; nothing restored", v)
			}
		}
	}
	if a := o.args(); len(a) > 0 {
		fmt.Fprintf(out, "Additional restore flags: %s\n", strings.Join(a, " "))
	}
	if len(o.Paths) > 0 {
		fmt.Fprintf(out, "Restoring only: %s\n", strings.Join(o.Paths, ", "))
	}
	// NO_PREVIEW=1 skips the preview; only a person at a terminal is asked to confirm, so
	// piped answers keep their order.
	if e.getenv("NO_PREVIEW") == "" {
		files, err := e.listFiles(scratch, id, rev)
		if err != nil {
			return 1, err
		}
		plan(files, dir, o).show(out, dir)
		if isTerminal(e.Stdin) {
			for {
				a, err := ask("Restore now? (Y/n): ")
				if err != nil {
					return 1, err
				}
				a = strings.ToLower(a)
				if a == "" || a == "y" || a == "yes" {
					break
				}
				if a == "n" || a == "no" {
					fmt.Fprintln(out, "Nothing restored.")
					return 0, nil
				}
				fmt.Fprintln(out, "Answer y or n.")
			}
		}
	}

	if dir, err = prepare(dir); err != nil {
		return 1, fmt.Errorf("cannot create the restore directory: %w", err)
	}
	// Again now: a scheduled backup may have started while the questions were answered.
	if err := e.guard(dir); err != nil {
		return 1, err
	}
	if err := e.connect(dir, t, id, out); err != nil {
		return 1, err
	}
	if err := e.restore(dir, t, id, rev, o, out); err != nil {
		return 1, fmt.Errorf("the restore from '%s' failed: %v", t.Name, err)
	}
	fmt.Fprintln(out, "Repository restored.")
	if e.AfterRestore != nil {
		e.AfterRestore(dir)
	}
	if !e.hasRestoreHook(dir) {
		return 0, nil
	}
	// RUN_RESTORE_SERVICE answers the question, so a scripted restore matches auto-restore.
	if e.getenv("RUN_RESTORE_SERVICE") == "" {
		fmt.Fprint(out, "\nThis directory has a restore hook. Run it now? (y/N): ")
		if !yes(in) {
			fmt.Fprintln(out, "Did not run the restore hook.")
			return 0, nil
		}
	}
	// The hook reads on from any answers already buffered (piped input), not past them. With
	// nothing buffered it gets stdin itself: a reader would make the hook's end wait on a
	// copy blocked reading the terminal.
	stdin := e.Stdin
	if n := in.Buffered(); n > 0 {
		ahead, _ := in.Peek(n)
		stdin = io.MultiReader(bytes.NewReader(ahead), e.Stdin)
	}
	code, err := e.postRestore(dir, id, rev, t, stdin, out)
	if err != nil {
		return 1, err
	}
	if code != 0 {
		return 1, fmt.Errorf("the restore hook exited %d", code)
	}
	return 0, nil
}

// serviceDirOf is the configured service directory whose snapshot ID is id, or "".
func (e *Env) serviceDirOf(id string) string {
	dirs, _ := config.ExpandServiceDirectories(e.cfg.ServiceDirectories)
	for _, d := range dirs {
		if e.Hostname+"-"+filepath.Base(d) == id {
			return d
		}
	}
	return ""
}

// isTerminal reports whether r is a terminal.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
