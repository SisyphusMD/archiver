package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/pipeline"
	"github.com/SisyphusMD/archiver/internal/restore"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func restoreEnv() *restore.Env {
	return &restore.Env{
		Layout:       layout.Default(),
		Source:       config.FromEnvironment(),
		Environ:      os.Environ(),
		Hostname:     pipeline.Hostname(os.Getenv),
		Duplicacy:    "duplicacy",
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		CapEff:       capEff(),
		AfterRestore: migrateRestored,
	}
}

// capEff reads the effective capability set, or -1 if it cannot.
func capEff() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return -1
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "CapEff:"); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 16, 64); err == nil {
				return n
			}
		}
	}
	return -1
}

// migrateRestored migrates a service-backup-settings.sh that a restore brought back into a
// configured service directory, when executable hooks (any service's) keep the deployment
// on the Go pipeline, which refuses the file. Without them the bash pipeline runs it as
// before, so it is kept; so is one restored elsewhere (a drill's scratch directory must stay
// exactly what was backed up). A failure only warns: the files are restored either way.
func migrateRestored(dir string) {
	if _, err := os.Lstat(filepath.Join(dir, hooks.Legacy)); err != nil {
		return
	}
	src := config.FromEnvironment()
	cfg, _, err := config.Load(src, nil)
	if err != nil {
		return
	}
	dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	configured, executable := false, false
	target, _ := filepath.Abs(dir)
	for _, d := range dirs {
		// The destination's own surviving hooks count too: they put the deployment on Go
		// just the same, and migration then reports the conflict.
		if abs, _ := filepath.Abs(d); abs == target {
			configured = true
		}
		for _, f := range []string{hooks.PreBackup, hooks.PostBackup, hooks.Filters} {
			if exists(filepath.Join(d, f)) {
				executable = true
			}
		}
	}
	if !configured {
		return
	}
	switch src.Getenv("ARCHIVER_PIPELINE") {
	case "bash":
		return
	case "go":
	default:
		if !executable {
			return
		}
	}
	l := layout.Default()
	lock, _, err := runlock.Acquire(l.BackupLock(), filepath.Join(l.Lock, "archiver-stop-requested"), "migrate", "hooks")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %s holds a restored %s, which backups refuse, and it could not be migrated now (%v). Run 'archiver migrate hooks %s'.\n", dir, hooks.Legacy, err, dir)
		return
	}
	defer lock.Release()
	m, err := hooks.Migrate(dir, pipeline.Hostname(os.Getenv))
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: the restored %s could not be migrated: %v. Backups refuse this service until 'archiver migrate hooks %s' succeeds.\n", hooks.Legacy, err, dir)
		return
	}
	fmt.Printf("%s: migrated the restored %s: wrote %v; kept the old file as %s.\n", dir, hooks.Legacy, m.Wrote, hooks.LegacyKept)
	for _, w := range m.Warnings {
		fmt.Fprintln(os.Stderr, "WARNING:", w)
	}
}
