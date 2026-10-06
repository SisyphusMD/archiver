// Package proc runs the programs a pipeline drives (duplicacy, hooks) as direct children,
// with their output logged line by line. They must stay direct children: `archiver pause`
// and `archiver stop` signal the pipeline's children (pkill -P), so anything started in
// between would escape them.
package proc

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/logging"
)

// Spec is one program to run.
type Spec struct {
	Path    string
	Args    []string
	Dir     string
	Env     []string // the complete environment
	Log     *logging.Log
	Service string // the log's service label
	// Output, when set, receives stdout and stderr as they are instead of the log.
	Output io.Writer
	// Hook logs each output line at the level its [ERROR] or [WARNING] prefix asks for.
	Hook bool
	// Group runs the program in its own process group, so Terminate ends everything it
	// started, however deep (a shell's command substitutions included).
	Group bool
}

// outputGrace is how long output is still read after the program itself exits.
var outputGrace = 2 * time.Second

// Proc is a started program.
type Proc struct {
	cmd   *exec.Cmd
	done  chan struct{}
	code  int
	err   error
	group bool
}

// Start starts the program with stdout and stderr logged as INFO lines, in order.
func Start(s Spec) (*Proc, error) {
	cmd := exec.Command(s.Path, s.Args...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	var w io.WriteCloser
	switch {
	case s.Output != nil:
		w = nopCloser{s.Output}
	case s.Hook:
		w = s.Log.HookWriter(s.Service)
	default:
		w = s.Log.Writer(logging.Info, s.Service)
	}
	cmd.Stdout, cmd.Stderr = w, w
	// A background process the program left running holds its output open; past this,
	// the program counts as done and the output pipe is closed.
	cmd.WaitDelay = outputGrace
	if s.Group {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		w.Close()
		return nil, err
	}
	p := &Proc{cmd: cmd, done: make(chan struct{}), group: s.Group}
	go func() {
		err := cmd.Wait()
		w.Close()
		p.code, p.err = code(err)
		close(p.done)
	}()
	return p, nil
}

// Run starts the program and waits for it.
func Run(s Spec) (int, error) {
	p, err := Start(s)
	if err != nil {
		return -1, err
	}
	return p.Wait()
}

// Wait waits for the program and returns its exit code; err is set only when it could not
// be waited for at all. A program killed by a signal returns 128 plus the signal number.
func (p *Proc) Wait() (int, error) {
	<-p.done
	return p.code, p.err
}

// Done is closed when the program has ended and its output is logged.
func (p *Proc) Done() <-chan struct{} { return p.done }

func code(err error) (int, error) {
	// ErrWaitDelay: it exited 0, and a background child it left kept its output open past
	// outputGrace. A failed exit is reported as an ExitError instead.
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	return -1, err
}

// PID is the program's process ID.
func (p *Proc) PID() int { return p.cmd.Process.Pid }

// Terminate ends the program: TERM to its children and itself, then
// KILL after a two-second grace. A paused (stopped) program cannot handle TERM, so a caller
// that knows the run is paused passes kill to go straight to KILL.
func (p *Proc) Terminate(kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	p.signal(sig)
	if kill {
		return
	}
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		p.signal(syscall.SIGKILL)
	}
}

func (p *Proc) signal(sig syscall.Signal) {
	if p.group {
		syscall.Kill(-p.PID(), sig)
		return
	}
	signalTree(p.PID(), sig)
}

// Signal sends sig to the program's children and the program, as pause and resume do.
func (p *Proc) Signal(sig syscall.Signal) { p.signal(sig) }

func signalTree(pid int, sig syscall.Signal) {
	for _, c := range Children(pid) {
		syscall.Kill(c, sig)
	}
	syscall.Kill(pid, sig)
}

// Children lists the direct children of pid, as pkill -P does.
func Children(pid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := strconv.Itoa(pid)
	var out []int
	for _, e := range entries {
		child, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:])
		if len(f) > 1 && f[1] == want {
			out = append(out, child)
		}
	}
	return out
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
