// Package lockstate reads the state the bash pipelines keep on disk: their lock files and
// the per-storage maintenance record. It never writes either; until the pipelines move to
// Go (ADR 10), bash owns both formats.
package lockstate

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Lock is one pipeline's lock file. Line 1 is "<pid> <context> <stage>"; every later line is
// "<epoch> <state>", appended as the run starts, pauses, resumes, and ends.
type Lock struct {
	PID     int
	Context string
	Stage   string
	Events  []Event
}

// Event is one state change of a run.
type Event struct {
	At    int64
	State string
}

// ReadLock parses the lock at path. ok is false when there is no lock file.
func ReadLock(path string) (lock Lock, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Lock{}, false, nil
	}
	if err != nil {
		return Lock{}, false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if first {
			first = false
			if len(fields) > 0 {
				lock.PID, _ = strconv.Atoi(fields[0])
			}
			if len(fields) > 1 {
				lock.Context = fields[1]
			}
			if len(fields) > 2 {
				lock.Stage = fields[2]
			}
			continue
		}
		if len(fields) < 2 {
			continue
		}
		at, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		lock.Events = append(lock.Events, Event{At: at, State: fields[1]})
	}
	return lock, true, sc.Err()
}

// Alive reports whether the lock's holder is still running. A lock whose holder died is
// stale: the next run of that pipeline reaps it.
func (l Lock) Alive() bool {
	if l.PID <= 0 {
		return false
	}
	err := syscall.Kill(l.PID, 0)
	// EPERM still proves the process exists.
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Paused reports whether the run's latest state change was a pause.
func (l Lock) Paused() bool {
	return len(l.Events) > 0 && l.Events[len(l.Events)-1].State == "paused"
}

// StartedAt is when the run took the lock, or 0 if the lock records no start.
func (l Lock) StartedAt() int64 {
	if len(l.Events) == 0 {
		return 0
	}
	return l.Events[0].At
}

// Storage is one line of the maintenance record: when check, prune, and an exhaustive
// prune last succeeded on a storage, as epoch seconds (0 = never).
type Storage struct {
	Name                     string
	Check, Prune, Exhaustive int64
}

// ReadMaintenance parses the maintenance record. A missing record is no storages.
func ReadMaintenance(path string) ([]Storage, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Storage
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		s := Storage{Name: fields[0]}
		for i, dst := range []*int64{&s.Check, &s.Prune, &s.Exhaustive} {
			if len(fields) > i+1 {
				*dst, _ = strconv.ParseInt(fields[i+1], 10, 64)
			}
		}
		out = append(out, s)
	}
	return out, sc.Err()
}
