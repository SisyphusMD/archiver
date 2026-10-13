package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/entrypoint"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func TestShutdownWaitsForDeepCheckProgress(t *testing.T) {
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: root}
	lock, _, err := runlock.Acquire(l.DeepCheckLock("local"), l.DeepCheckStopFlag("local"), "deep-check", "local")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	child := filepath.Join(root, "duplicacy")
	ready, saved := filepath.Join(root, "ready"), filepath.Join(root, "verified_chunks")
	script := "#!/bin/sh\ntrap 'sleep 1.2; printf saved > " + saved + "; exit 130' INT\ntouch " + ready + "\nwhile :; do sleep 0.05; done\n"
	if err := os.WriteFile(child, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := proc.Start(proc.Spec{Path: child, Output: io.Discard, Interrupt: true})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Terminate(true)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop := filepath.Join(root, "stop")
	if err := os.WriteFile(stop, []byte("#!/bin/sh\nkill -INT "+strconv.Itoa(p.PID())+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = p.Wait(); lock.Release(); close(done) }()
	tailers := make(chan struct{})
	shutdown(&entrypoint.Env{Layout: l, Self: stop}, tailers)
	_, statErr := os.Stat(saved)
	// Join cleanup even when shutdown returned too early.
	<-done
	if statErr != nil {
		t.Fatal("shutdown exited before the deep check saved verified chunks")
	}
	select {
	case <-tailers:
	default:
		t.Fatal("shutdown did not stop tailers")
	}
	e := &entrypoint.Env{Layout: l}
	if !e.LocksClear() {
		t.Fatal("released deep-check flock still blocks shutdown")
	}
}
