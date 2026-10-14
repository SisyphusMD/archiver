package daemon

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var drillName = regexp.MustCompile(`^[A-Z0-9_]+$`)

var drillVariable = regexp.MustCompile(`^RESTORE_DRILL_([A-Z0-9_]+)_(SCHEDULE|SERVICES|STORAGES|TIMEOUT)$`)

// NamedDrillName excludes global setting names, which would make setting names ambiguous.
func NamedDrillName(name string) bool {
	if !drillName.MatchString(name) {
		return false
	}
	switch name {
	case "SERVICES", "STORAGES", "DIR", "EXCLUDE", "TIMEOUT", "HOOK", "HOOK_TIMEOUT", "HOOK_GRACE", "SCHEDULE":
		return false
	}
	return true
}

// NamedDrillNames discovers definitions from environment keys, including incomplete ones so typos fail early.
func NamedDrillNames(environ []string) ([]string, error) {
	names := map[string]bool{}
	globals := map[string]bool{"RESTORE_DRILL_SCHEDULE": true, "RESTORE_DRILL_SERVICES": true, "RESTORE_DRILL_STORAGES": true, "RESTORE_DRILL_TIMEOUT": true, "RESTORE_DRILL_HOOK_TIMEOUT": true, "RESTORE_DRILL_HOOK_GRACE": true, "RESTORE_DRILL_DIR": true, "RESTORE_DRILL_EXCLUDE": true}
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, "RESTORE_DRILL_") || globals[key] {
			continue
		}
		m := drillVariable.FindStringSubmatch(key)
		if m == nil || !NamedDrillName(m[1]) {
			return nil, fmt.Errorf("invalid named restore drill setting %s: expected RESTORE_DRILL_<NAME>_{SCHEDULE,SERVICES,STORAGES,TIMEOUT}; NAME must match [A-Z0-9_]+ and must not be a reserved setting name", key)
		}
		names[m[1]] = true
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// IsNamedDrillSetting recognizes dynamic settings when recording recovery configuration.
func IsNamedDrillSetting(key string) bool {
	m := drillVariable.FindStringSubmatch(key)
	return m != nil && NamedDrillName(m[1])
}
