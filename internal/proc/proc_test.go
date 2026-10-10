package proc

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/logging"
)

// A background process the program leaves holding its output must not keep the program
// counted as running.
func TestBackgroundChildDoesNotHoldWait(t *testing.T) {
	dir := t.TempDir()
	log := &logging.Log{Dir: dir, Basename: "archiver"}
	began := time.Now()
	code, err := Run(Spec{Path: "/bin/sh", Args: []string{"-c", "sleep 30 & echo started; exit 4"}, Env: os.Environ(), Log: log, Service: "svc"})
	if err != nil || code != 4 {
		t.Fatalf("code %d, err %v", code, err)
	}
	// A successful exit stays successful when the child outlives the output grace.
	if code, err := Run(Spec{Path: "/bin/sh", Args: []string{"-c", "sleep 30 & exit 0"}, Env: os.Environ(), Log: log}); err != nil || code != 0 {
		t.Fatalf("successful hook with a background child: code %d, err %v", code, err)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("waited %v for a background child", took)
	}
	b, _ := os.ReadFile(log.Path())
	if !strings.Contains(string(b), "[Service: svc] started") {
		t.Fatalf("output not logged:\n%s", b)
	}
}

func TestTerminate(t *testing.T) {
	log := &logging.Log{Dir: t.TempDir(), Basename: "archiver"}
	p, err := Start(Spec{Path: "/bin/sh", Args: []string{"-c", "trap '' TERM; sleep 30"}, Env: os.Environ(), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	began := time.Now()
	p.Terminate(false) // TERM ignored, so KILL after the grace
	code, _ := p.Wait()
	if code != 128+9 || time.Since(began) > 10*time.Second {
		t.Fatalf("code %d after %v", code, time.Since(began))
	}
}

// An Interrupt program is sent INT first and given time to save its state.
func TestTerminateInterrupts(t *testing.T) {
	log := &logging.Log{Dir: t.TempDir(), Basename: "archiver"}
	saved := t.TempDir() + "/saved"
	p, err := Start(Spec{Path: "/bin/sh", Args: []string{"-c", "trap 'sleep 1; echo ok > " + saved + "; exit 1' INT; trap '' TERM; while :; do sleep 0.1; done"}, Env: os.Environ(), Log: log, Interrupt: true})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	p.Terminate(false)
	if code, _ := p.Wait(); code != 1 {
		t.Fatalf("code %d: the INT handler did not end it", code)
	}
	if b, _ := os.ReadFile(saved); string(b) != "ok\n" {
		t.Fatal("the INT handler did not run to completion")
	}
	// One that ignores INT still ends.
	defer func(g time.Duration) { interruptGrace = g }(interruptGrace)
	interruptGrace = 300 * time.Millisecond
	p, _ = Start(Spec{Path: "/bin/sh", Args: []string{"-c", "trap '' INT; sleep 30"}, Env: os.Environ(), Log: log, Interrupt: true})
	time.Sleep(200 * time.Millisecond)
	p.Terminate(false)
	if code, _ := p.Wait(); code != 128+15 {
		t.Fatalf("code %d", code)
	}
}

// Terminating a group ends grandchildren too.
func TestTerminateGroup(t *testing.T) {
	log := &logging.Log{Dir: t.TempDir(), Basename: "archiver"}
	pidFile := t.TempDir() + "/grandchild"
	p, err := Start(Spec{Path: "/bin/sh", Args: []string{"-c", "sh -c 'echo $$ > " + pidFile + "; sleep 30' & wait"}, Env: os.Environ(), Log: log, Group: true})
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
	}
	if pid == 0 {
		t.Fatal("grandchild never started")
	}
	p.Terminate(false)
	p.Wait()
	time.Sleep(200 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil && !zombie(pid) {
		t.Fatal("the grandchild outlived Terminate")
	}
}

func zombie(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i >= 0 && i+2 < len(s) && s[i+2] == 'Z'
}
