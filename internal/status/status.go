// Package status implements `archiver status`: whether each pipeline is running, and when
// maintenance last succeeded on each storage.
package status

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/SisyphusMD/archiver/internal/backuphealth"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/envelope"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/notify"
)

// Write prints the status report. It fails only when a state file exists but cannot be read.
func Write(w io.Writer, l layout.Layout, getenv func(string) string, now time.Time) error {
	h := backuphealth.Compute(l, getenv, now)
	fmt.Fprintf(w, "Backup health: %s\n", h.State)
	for _, r := range h.Reasons {
		fmt.Fprintf(w, "  %s\n", r)
	}
	backup, held, err := lockstate.ReadLock(l.BackupLock())
	if err != nil {
		return err
	}
	switch {
	case !held || !backup.Alive():
		fmt.Fprintln(w, "Backup: not running.")
	case backup.Paused():
		fmt.Fprintf(w, "Backup: paused (PID: %d).\n", backup.PID)
	default:
		fmt.Fprintf(w, "Backup: running (PID: %d, stage: %s).\n", backup.PID, backup.Stage)
	}

	maint, held, err := lockstate.ReadLock(l.MaintenanceLock())
	if err != nil {
		return err
	}
	if held && maint.Alive() {
		fmt.Fprintf(w, "Maintenance: running (PID: %d, stage: %s, %s).\n", maint.PID, maint.Stage, maint.Context)
	} else {
		fmt.Fprintln(w, "Maintenance: not running.")
	}

	if d, held, err := lockstate.ReadLock(l.DrillLock()); err == nil && held && d.Alive() {
		state := "running"
		if d.Paused() {
			state = "paused"
		}
		fmt.Fprintf(w, "Restore drill: %s (PID: %d, %s).\n", state, d.PID, d.Stage)
	}

	if states := (&copier.Store{Path: l.CopyWorkersState()}).Load(); len(states) > 0 {
		fmt.Fprintln(w, "Copies (as of the daemon's last update):")
		names := make([]string, 0, len(states))
		for n := range states {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(w, "  %s: %s\n", n, describe(states[n], now))
		}
	}

	if line, _ := envelope.Status(l, now, Age); line != "" {
		fmt.Fprintln(w, line)
	}

	if ds, err := lockstate.ReadDrillState(l.DrillState()); err == nil && len(ds.Results) > 0 {
		fmt.Fprintln(w, "Restore drills (last per service):")
		storagesDrilled := make([]string, 0, len(ds.Results))
		for s := range ds.Results {
			storagesDrilled = append(storagesDrilled, s)
		}
		sort.Strings(storagesDrilled)
		for _, s := range storagesDrilled {
			services := make([]string, 0, len(ds.Results[s]))
			for n := range ds.Results[s] {
				services = append(services, n)
			}
			sort.Strings(services)
			for _, n := range services {
				r := ds.Results[s][n]
				what := "passed"
				switch {
				case r.Skipped:
					what = "skipped (" + r.Message + ")"
				case !r.OK:
					what = "FAILED (" + r.Message + ")"
				}
				fmt.Fprintf(w, "  %s on %s: revision %d %s %s\n", n, s, r.Revision, what, Age(r.At, now))
			}
		}
	}

	storages, err := lockstate.ReadMaintenance(l.MaintenanceState())
	if err != nil {
		return err
	}
	if len(storages) > 0 {
		fmt.Fprintln(w, "Storage maintenance (last success):")
		for _, s := range storages {
			fmt.Fprintf(w, "  %s: check %s, prune %s, exhaustive %s\n",
				s.Name, Age(s.Check, now), Age(s.Prune, now), Age(s.Exhaustive, now))
		}
	}
	return nil
}

// describe is one copy worker's line.
func describe(s copier.State, now time.Time) string {
	var line string
	switch s.Status {
	case copier.Idle:
		line = "caught up, last copy " + Age(s.LastSuccess, now)
	case copier.Copying:
		line = fmt.Sprintf("copying %d revisions, started %s", s.Behind, Age(s.Since, now))
	case copier.Retrying:
		line = fmt.Sprintf("retrying %s, failing since %s: %s", until(s.NextRetry, now), Age(s.FailingSince, now), s.LastError)
	case copier.Down:
		line = fmt.Sprintf("DOWN since %s, retrying %s: %s", Age(s.DownSince, now), until(s.NextRetry, now), s.LastError)
	case copier.Stopped:
		line = "stopped; copies again after the next backup"
	case copier.Held:
		line = fmt.Sprintf("waiting for its copy window (%s), which opens %s", s.Window, until(s.HeldUntil, now))
		if s.Behind > 0 {
			line += fmt.Sprintf("; %d revisions behind", s.Behind)
		}
	case copier.Mirroring:
		line = "deleting revisions local has pruned, started " + Age(s.Since, now)
	case copier.Pruning:
		line = "exhaustive prune, started " + Age(s.Since, now)
	case copier.Checking:
		line = "checking, started " + Age(s.Since, now)
	default:
		line = s.Status
	}
	if s.HeldUntil != 0 && s.Status != copier.Held {
		line += fmt.Sprintf("; held until its copy window (%s) opens %s", s.Window, until(s.HeldUntil, now))
	}
	if s.CheckEvery > 0 {
		switch {
		case s.CheckFailed != "":
			line += "; LAST CHECK FAILED"
		case s.LastCheck == 0:
			line += "; not checked yet"
		case now.Unix()-s.LastCheck > 2*s.CheckEvery:
			line += "; check OVERDUE, last " + Age(s.LastCheck, now)
		default:
			line += "; checked " + Age(s.LastCheck, now)
		}
	}
	if s.MirrorRefused != "" {
		line += "; MIRROR REFUSED: " + s.MirrorRefused
	}
	if s.Status == copier.Copying && s.DownSince != 0 {
		line += ", DOWN since " + Age(s.DownSince, now)
	}
	if s.Paused {
		line += " (paused)"
	}
	return line
}

// until renders how soon an epoch is.
func until(epoch int64, now time.Time) string {
	d := epoch - now.Unix()
	switch {
	case d <= 0:
		return "now"
	case d < 3600:
		return fmt.Sprintf("in %dm", (d+59)/60)
	default:
		return fmt.Sprintf("in %dh", (d+3599)/3600)
	}
}

// Age renders how long ago an epoch was, coarsely: minutes under an hour, hours under two
// days, then days. 0 means it never happened.
func Age(epoch int64, now time.Time) string {
	if epoch == 0 {
		return "never"
	}
	d := now.Unix() - epoch
	switch {
	case d < 3600:
		return fmt.Sprintf("%dm ago", d/60)
	case d < 172800:
		return fmt.Sprintf("%dh ago", d/3600)
	default:
		return fmt.Sprintf("%dd ago", d/86400)
	}
}

// Run is one run's lock, for JSON.
type Run struct {
	Running bool   `json:"running"`
	Paused  bool   `json:"paused,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Context string `json:"context,omitempty"`
}

// Snapshot is everything status shows, for monitors and the web page (ADRs 32, 38).
type Snapshot struct {
	At           int64                                       `json:"at"`
	BackupHealth backuphealth.Health                         `json:"backup_health"`
	Backup       Run                                         `json:"backup"`
	Maintenance  Run                                         `json:"maintenance"`
	Drill        Run                                         `json:"drill"`
	Services     map[string]lockstate.ServiceResult          `json:"services"`
	Copies       map[string]copier.State                     `json:"copies"`
	Drills       map[string]map[string]lockstate.DrillResult `json:"drills"`
	Storages     []lockstate.Storage                         `json:"storages"`
	Incidents    map[string]string                           `json:"incidents"`
	Envelope     string                                      `json:"envelope,omitempty"`
}

// Take reads the snapshot.
func Take(l layout.Layout, getenv func(string) string, now time.Time) Snapshot {
	s := Snapshot{At: now.Unix(), BackupHealth: backuphealth.Compute(l, getenv, now)}
	run := func(path string) Run {
		lk, held, err := lockstate.ReadLock(path)
		if err != nil || !held || !lk.Alive() {
			return Run{}
		}
		return Run{Running: true, Paused: lk.Paused(), PID: lk.PID, Stage: lk.Stage, Context: lk.Context}
	}
	s.Backup, s.Maintenance, s.Drill = run(l.BackupLock()), run(l.MaintenanceLock()), run(l.DrillLock())
	bs, _ := lockstate.ReadBackupState(l.BackupState())
	s.Services = bs.Services
	s.Copies = (&copier.Store{Path: l.CopyWorkersState()}).Load()
	if ds, err := lockstate.ReadDrillState(l.DrillState()); err == nil {
		s.Drills = ds.Results
	}
	s.Storages, _ = lockstate.ReadMaintenance(l.MaintenanceState())
	s.Incidents = notify.OpenIncidents(l.Incidents())
	s.Envelope, _ = envelope.Status(l, now, Age)
	return s
}

// WriteJSON writes the snapshot as JSON.
func WriteJSON(w io.Writer, l layout.Layout, getenv func(string) string, now time.Time) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Take(l, getenv, now))
}
