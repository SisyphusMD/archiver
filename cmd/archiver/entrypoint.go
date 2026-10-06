package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/entrypoint"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// entrypointCommand is the container's entrypoint: `archiver entrypoint [init | run CMD...]`.
func entrypointCommand(args []string) int {
	l := layout.Default()
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
		if !ready() {
			return 1
		}
		os.Chdir(l.Root)
		err := e.Exec(args[1:]...)
		fmt.Fprintln(os.Stderr, "archiver: cannot run", args[1]+":", err)
		return 127
	}

	if !ready() {
		return 1
	}
	e.ClearLocks()

	stopTailers := make(chan struct{})
	if fi, err := os.Stat(l.LogDir()); err == nil && fi.IsDir() {
		if logo, err := os.ReadFile(l.Logo()); err == nil {
			os.Stdout.Write(logo)
			fmt.Println()
		}
		for _, t := range []struct{ file, banner string }{
			{"archiver.log", "Archiver Logs"}, {"maintenance.log", "Maintenance Logs"}, {"copies.log", "Copy Logs"},
		} {
			go entrypoint.Follow(filepath.Join(l.LogDir(), t.file), t.banner, os.Stdout, stopTailers)
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
	backup, maint := os.Getenv("BACKUP_SCHEDULE"), os.Getenv("MAINTENANCE_SCHEDULE")
	if backup != "" || maint != "" {
		if backup != "" {
			fmt.Println("Backups scheduled: " + backup)
		}
		if maint != "" {
			fmt.Println("Maintenance scheduled: " + maint)
		}
		// Fail fast on a malformed schedule instead of crash-looping the container.
		if runDaemon([]string{"--check"}) != 0 {
			fmt.Println("ERROR: BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid.")
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
	fmt.Println("No BACKUP_SCHEDULE set. Container will wait for manual commands.")
	fmt.Println("Use 'docker exec <container> archiver backup' to run backups manually ('archiver backup --detach' to background)")
	fmt.Println()
	fmt.Println("Container is ready and will stay running.")
	<-sigs
	shutdown(e, stopTailers)
	return 0
}

// shutdown stops whatever runs and waits for both pipelines to record the stop and release
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
