package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/daemon"
)

// NamedDrill always selects its complete service list, independently of regular exclusions.
type NamedDrill struct {
	Name, Schedule, Storages string
	Services                 []string
	Timeout                  time.Duration
}

func (c *Config) loadNamedDrills(src Source, environ []string) ([]NamedDrill, error) {
	names, err := daemon.NamedDrillNames(environ)
	if err != nil {
		return nil, err
	}
	var out []NamedDrill
	for _, name := range names {
		p := "RESTORE_DRILL_" + name + "_"
		d := NamedDrill{Name: name, Schedule: src.Getenv(p + "SCHEDULE"), Storages: src.Getenv(p + "STORAGES")}
		if d.Schedule == "" {
			return nil, fmt.Errorf("invalid %sSCHEDULE: a named drill requires a schedule", p)
		}
		for _, raw := range strings.Split(src.Getenv(p+"SERVICES"), ",") {
			service := strings.TrimSpace(raw)
			if service == "" {
				return nil, fmt.Errorf("invalid %sSERVICES: required nonempty comma-separated service names", p)
			}
			if !slices.Contains(d.Services, service) {
				d.Services = append(d.Services, service)
			}
		}
		if d.Storages == "" {
			d.Storages = "all"
		}
		d.Storages = strings.TrimSpace(d.Storages)
		if targets, err := d.Targets(c); err != nil {
			return nil, fmt.Errorf("invalid %sSTORAGES: %w", p, err)
		} else if len(targets) == 0 {
			return nil, fmt.Errorf("invalid %sSTORAGES: no storages selected", p)
		}
		copy := *c
		copy.DrillLimit = src.Getenv(p + "TIMEOUT")
		d.Timeout, err = copy.DrillTimeout()
		if err != nil {
			return nil, fmt.Errorf("invalid %sTIMEOUT: %w", p, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func (c *Config) NamedDrill(name string) (NamedDrill, bool) {
	for _, d := range c.NamedDrills {
		if d.Name == name {
			return d, true
		}
	}
	return NamedDrill{}, false
}

// ValidateNamedDrills checks live service membership separately because a bare recovery host has not restored its directories yet.
func (c *Config) ValidateNamedDrills() error {
	dirs, _ := ExpandServiceDirectories(c.ServiceDirectories)
	var configured []string
	for _, dir := range dirs {
		configured = append(configured, filepath.Base(dir))
	}
	for _, d := range c.NamedDrills {
		if err := daemon.ValidSchedule(d.Schedule, time.Now()); err != nil {
			return fmt.Errorf("invalid RESTORE_DRILL_%s_SCHEDULE: %w", d.Name, err)
		}
		for _, service := range d.Services {
			if !slices.Contains(configured, service) {
				return fmt.Errorf("invalid RESTORE_DRILL_%s_SERVICES: %q is not a configured service (SERVICE_DIRECTORIES)", d.Name, service)
			}
		}
	}
	return nil
}

// Alternating reports whether the drill visits one storage per run.
func (d NamedDrill) Alternating() bool {
	return d.Storages == "alternate" || strings.HasPrefix(d.Storages, "alternate:")
}

// Targets resolves named selections in their declared order.
func (d NamedDrill) Targets(c *Config) ([]Target, error) {
	selection := *c
	selection.DrillStorages = d.Storages
	if d.Storages == "alternate" {
		if len(c.Targets) == 0 {
			return nil, fmt.Errorf("no storages configured")
		}
		selection.DrillStorages = "all"
	} else if list, ok := strings.CutPrefix(d.Storages, "alternate:"); ok {
		var targets []Target
		for _, raw := range strings.Split(list, ",") {
			name := strings.TrimSpace(raw)
			if name == "" {
				return nil, fmt.Errorf("required nonempty comma-separated storage names")
			}
			found := false
			for _, target := range c.Targets {
				if target.Name == name {
					targets = append(targets, target)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("RESTORE_DRILL_STORAGES names '%s', which is not a configured storage (STORAGE_TARGET_N_NAME).", name)
			}
		}
		return targets, nil
	}
	return selection.DrillTargets()
}
