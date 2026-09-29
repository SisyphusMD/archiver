// Package layout names the files archiver keeps inside the image. The bash implementation
// declares the same paths in lib/core/common.sh; while both run side by side they must agree.
package layout

import "path/filepath"

// Layout is every path the Go commands read. Tests point Root and Lock at temp dirs.
type Layout struct {
	Root string // /opt/archiver
	Lock string // /var/lock
}

// Default is the layout inside the image.
func Default() Layout { return Layout{Root: "/opt/archiver", Lock: "/var/lock"} }

func (l Layout) LogDir() string           { return filepath.Join(l.Root, "logs") }
func (l Layout) BackupLog() string        { return filepath.Join(l.LogDir(), "archiver.log") }
func (l Layout) MaintenanceLog() string   { return filepath.Join(l.LogDir(), "maintenance.log") }
func (l Layout) MaintenanceState() string { return filepath.Join(l.LogDir(), ".maintenance-state") }
func (l Layout) ConfigFile() string       { return filepath.Join(l.Root, "config.sh") }
func (l Layout) RSAPrivateKey() string    { return filepath.Join(l.Root, "keys", "private.pem") }
func (l Layout) SSHPrivateKey() string    { return filepath.Join(l.Root, "keys", "id_ed25519") }
func (l Layout) Logo() string             { return filepath.Join(l.Root, "lib", "logos", "logo.ascii") }
func (l Layout) BackupLock() string       { return filepath.Join(l.Lock, "archiver-main.lock") }
func (l Layout) MaintenanceLock() string  { return filepath.Join(l.Lock, "archiver-maintenance.lock") }
