package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/entrypoint"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// entrypointCommand is the container's entrypoint: `archiver entrypoint [init | run CMD... |
// migrate hooks [DIR...]]`.
func entrypointCommand(args []string) int {
	l := layout.Default()
	// LOG_FORMAT=json covers the long-running container: everything it and its children
	// print goes through JSONLines. init and migrate are interactive, and run replaces this
	// process with the command, so those stay text.
	interactive := len(args) > 0 && (args[0] == "init" || args[0] == "migrate" || args[0] == "run" || args[0] == "recover")
	stdout := os.Stdout
	if os.Getenv("LOG_FORMAT") == "json" && !interactive {
		flush := jsonStdio()
		defer flush()
	}
	e := &entrypoint.Env{Layout: l, Getenv: os.Getenv, SecretsDir: config.FromEnvironment().SecretsDir,
		Stdout: os.Stdout, Stderr: os.Stderr, Self: selfPath}
	fmt.Println("===================================")
	fmt.Println("Archiver Container Starting")
	fmt.Println("===================================")

	if len(args) > 0 && args[0] == "init" {
		fmt.Println("Running in INIT mode")
		fmt.Println()
		setup := filepath.Join(l.Root, "setup")
		if _, err := os.Lstat(filepath.Join(setup, "env-native")); err == nil {
			fmt.Printf("WARNING: %s/env-native already exists\nContinuing will overwrite it.\n\n", setup)
		}
		os.MkdirAll(setup, 0o755)
		os.Chdir(l.Root)
		return initCommand()
	}

	// A fresh host has no keys or configuration yet: recovery brings them, so it runs before
	// anything that needs them.
	if len(args) > 0 && args[0] == "recover" {
		fmt.Println("Running in RECOVER mode")
		code, ok := recoverCommand(args[1:])
		if !ok {
			fmt.Fprintln(os.Stderr, "usage: recover KIT [--yes] [--hook] [--out DIR]")
			return 2
		}
		return code
	}

	// Converts legacy settings files, which keep the container from starting.
	if len(args) >= 2 && args[0] == "migrate" && args[1] == "hooks" {
		os.Chdir(l.Root)
		return migrateHooks(args[2:])
	}

	refuseLegacy := func() bool {
		if dirs := entrypoint.LegacyServices(config.FromEnvironment()); len(dirs) > 0 {
			fmt.Fprint(os.Stderr, entrypoint.LegacyHelp(dirs))
			return true
		}
		return false
	}

	ready := func() bool {
		if found := e.RefuseBundle(); found != "" {
			fmt.Fprintf(os.Stderr, "ERROR: found %s.\n%s", found, entrypoint.BundleHelp)
			return false
		}
		if err := e.PlaceKeys(); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR:", err)
			return false
		}
		fmt.Println("Configuration: keys loaded from files.")
		return true
	}

	if len(args) > 0 && args[0] == "run" {
		if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "ERROR: 'run' requires a subcommand (e.g., 'run snapshot-exists')")
			return 2
		}
		if !slices.Contains(entrypoint.RunCommands, args[1]) {
			fmt.Fprintf(os.Stderr, "ERROR: 'run' only supports: %s\nReceived: %s\n", strings.Join(entrypoint.RunCommands, ", "), args[1])
			return 2
		}
		fmt.Printf("Running in RUN mode: %s\n\n", strings.Join(args[1:], " "))
		if !ready() || (args[1] == "backup" && refuseLegacy()) {
			return 1
		}
		os.Chdir(l.Root)
		err := e.Exec(args[1:]...)
		fmt.Fprintln(os.Stderr, "archiver: cannot run", args[1]+":", err)
		return 127
	}

	if !ready() || refuseLegacy() {
		return 1
	}
	e.ClearLocks()

	stopTailers := make(chan struct{})
	jsonLogs := os.Getenv("LOG_FORMAT") == "json" && !interactive
	if fi, err := os.Stat(l.LogDir()); err == nil && fi.IsDir() {
		if logo, err := os.ReadFile(l.Logo()); err == nil && !jsonLogs {
			os.Stdout.Write(logo)
			fmt.Println()
		}
		for _, t := range []struct{ file, banner, log string }{
			{"archiver.log", "Archiver Logs", "archiver"}, {"maintenance.log", "Maintenance Logs", "maintenance"}, {"copies.log", "Copy Logs", "copies"},
			{"drill.log", "Restore Drill Logs", "drill"},
		} {
			var w io.Writer = os.Stdout
			if jsonLogs {
				w = entrypoint.JSONLines(t.log, stdout)
			}
			go entrypoint.Follow(filepath.Join(l.LogDir(), t.file), t.banner, w, stopTailers)
		}
	}
	fatal, warnings := e.Warnings()
	if fatal != "" {
		fmt.Fprintln(os.Stderr, "ERROR:", fatal)
		return 1
	}
	for _, w := range warnings {
		fmt.Println(w)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	var daemon *exec.Cmd
	backup, maint, drill := os.Getenv("BACKUP_SCHEDULE"), os.Getenv("MAINTENANCE_SCHEDULE"), os.Getenv("RESTORE_DRILL_SCHEDULE")
	if backup != "" || maint != "" || drill != "" {
		if backup != "" {
			fmt.Println("Backups scheduled: " + backup)
		}
		if maint != "" {
			fmt.Println("Maintenance scheduled: " + maint)
		}
		if drill != "" {
			fmt.Println("Restore drills scheduled: " + drill)
		}
		// Fail fast on a malformed schedule instead of crash-looping the container.
		if runDaemon([]string{"--check"}) != 0 {
			fmt.Println("ERROR: BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid (or RESTORE_DRILL_SCHEDULE).")
			return 1
		}
		fmt.Println("Starting scheduler...")
		os.Chdir(l.Root)
		daemon = exec.Command(selfPath, "daemon")
		daemon.Stdout, daemon.Stderr = os.Stdout, os.Stderr
		if err := daemon.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR: cannot start the scheduler:", err)
			return 1
		}
		exited := make(chan error, 1)
		go func() { exited <- daemon.Wait() }()
		select {
		case err := <-exited:
			// The scheduler ended on its own: the container ends with it.
			if err != nil {
				return 1
			}
			return 0
		case <-sigs:
			shutdown(e, stopTailers)
			daemon.Process.Signal(syscall.SIGTERM)
			select {
			case <-exited:
			case <-time.After(20 * time.Second):
				daemon.Process.Kill()
			}
			return 0
		}
	}
	if err := config.CheckPorts(os.Getenv("METRICS_PORT"), os.Getenv("WEB_PORT")); err != nil {
		fmt.Println("ERROR:", err)
		return 1
	}
	// No daemon in manual mode: the metrics and the status page run here (ADRs 35, 38).
	metricsCtx, endMetrics := context.WithCancel(context.Background())
	defer endMetrics()
	startMetrics(metricsCtx, l)
	fmt.Println("No BACKUP_SCHEDULE set. Container will wait for manual commands.")
	fmt.Println("Use 'docker exec <container> archiver backup' to run backups manually ('archiver backup --detach' to background)")
	fmt.Println()
	fmt.Println("Container is ready and will stay running.")
	<-sigs
	shutdown(e, stopTailers)
	return 0
}

// shutdown stops whatever runs and waits for every run to record the stop and release
// their locks: exiting first would tear down the PID namespace and kill that cleanup midway.
// The wait is bounded well under the documented stop_grace_period of two minutes.
func shutdown(e *entrypoint.Env, stopTailers chan struct{}) {
	fmt.Println("Received shutdown signal, attempting graceful stop...")
	stop := exec.Command(selfPath, "stop")
	stop.Stdout, stop.Stderr = os.Stdout, os.Stdout
	stop.Run()
	for range 100 {
		if e.LocksClear() {
			break
		}
		time.Sleep(time.Second)
	}
	close(stopTailers)
}

// jsonStdio sends this process's stdout and stderr, and so its children's, through
// JSONLines as the "entrypoint" log (stderr lines as ERROR); flush waits, up to 5 seconds, until everything
// written has been printed.
func jsonStdio() (flush func()) {
	out := os.Stdout
	outR, outW, err1 := os.Pipe()
	errR, errW, err2 := os.Pipe()
	if err1 != nil || err2 != nil {
		return func() {}
	}
	os.Stdout, os.Stderr = outW, errW
	var wg sync.WaitGroup
	for _, p := range []struct {
		r     *os.File
		level string
	}{{outR, "INFO"}, {errR, "ERROR"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			io.Copy(entrypoint.JSONLinesAt("entrypoint", p.level, out), p.r)
		}()
	}
	return func() {
		outW.Close()
		errW.Close()
		// A child still running (the scheduler died mid-backup) holds the write ends open,
		// so the readers never see EOF; the container must exit anyway.
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
}
