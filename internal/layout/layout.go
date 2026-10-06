// Package layout names the files archiver keeps inside the image.
package layout

import "path/filepath"

// Layout is every path the Go commands read. Tests point Root and Lock at temp dirs.
type Layout struct {
	Root string // /opt/archiver
	Lock string // /var/lock
}

// Default is the layout inside the image.
func Default() Layout { return Layout{Root: "/opt/archiver", Lock: "/var/lock"} }

func (l Layout) LogDir() string            { return filepath.Join(l.Root, "logs") }
func (l Layout) BackupLog() string         { return filepath.Join(l.LogDir(), "archiver.log") }
func (l Layout) MaintenanceLog() string    { return filepath.Join(l.LogDir(), "maintenance.log") }
func (l Layout) MaintenanceState() string  { return filepath.Join(l.LogDir(), ".maintenance-state") }
func (l Layout) RSAPrivateKey() string     { return filepath.Join(l.Root, "keys", "private.pem") }
func (l Layout) SSHPrivateKey() string     { return filepath.Join(l.Root, "keys", "id_ed25519") }
func (l Layout) Logo() string              { return filepath.Join(l.Root, "lib", "logos", "logo.ascii") }
func (l Layout) BackupLock() string        { return filepath.Join(l.Lock, "archiver-main.lock") }
func (l Layout) MaintenanceLock() string   { return filepath.Join(l.Lock, "archiver-maintenance.lock") }
func (l Layout) DaemonSocket() string      { return filepath.Join(l.Lock, "archiver-daemon.sock") }
func (l Layout) CopyWorkersState() string  { return filepath.Join(l.LogDir(), ".copy-workers.json") }
func (l Layout) InUseDir() string          { return filepath.Join(l.Lock, "archiver-in-use") }
func (l Layout) EnvelopeConfirmed() string { return filepath.Join(l.LogDir(), ".envelope-confirmed") }
func (l Layout) EnvelopeCurrent() string   { return filepath.Join(l.LogDir(), ".envelope-current") }

// StorageInit is the lock held around duplicacy init or add of one storage: created by
// two at once, a storage can get two configurations (duplicacy 3.2.5 has no
// create-if-absent). One per storage, so a slow offsite never holds up another.
func (l Layout) StorageInit(storage string) string {
	return filepath.Join(l.Lock, "archiver-storage-init-"+storage+".flock")
}

// CopyLock is held around every copy into one storage, by its worker and by a backup
// copying inline alike, so a storage never takes two copies at once.
func (l Layout) CopyLock(storage string) string {
	return filepath.Join(l.Lock, "archiver-copy-"+storage+".flock")
}
