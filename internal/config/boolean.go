package config

import (
	"fmt"
	"strings"
)

// ParseBool parses an on/off environment value; name identifies invalid settings.
func ParseBool(name, value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true, nil
	case "", "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s: invalid boolean %q (use true/1/yes/on or false/0/no/off/empty)", name, value)
	}
}

// MaintenanceBooleans preserves the enabled defaults and the legacy prune alias.
func MaintenanceBooleans(getenv func(string) string) (check, prune bool, err error) {
	read := func(name string) (bool, error) {
		value := getenv(name)
		if value == "" {
			value = "true"
		}
		return ParseBool(name, value)
	}
	check, err = read("CHECK_BACKUPS")
	if err != nil {
		return false, false, err
	}
	name := "PRUNE_BACKUPS"
	if getenv(name) == "" && getenv("ROTATE_BACKUPS") != "" {
		name = "ROTATE_BACKUPS"
	}
	prune, err = read(name)
	return check, prune, err
}
