// Package status implements `archiver status`: whether each pipeline is running, and when
// maintenance last succeeded on each storage.
package status

import (
	"fmt"
	"io"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

// Write prints the status report. It fails only when a state file exists but cannot be read.
func Write(w io.Writer, l layout.Layout, now time.Time) error {
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
