package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ReferenceMarkdown is docs/configuration.md, generated from Reference and the storage types
// (ADR 43): `UPDATE_REFERENCE=1 go test ./internal/config` rewrites it.
func ReferenceMarkdown() string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# Configuration reference")
	w("")
	w("<!-- Generated from internal/config/reference.go by `UPDATE_REFERENCE=1 go test ./internal/config`. Do not edit. -->")
	w("")
	w("Settings are environment variables. **Secrets** are files, never environment variables: `<secrets dir>/<name in lower case>` (`/run/secrets/storage_password`), or the file `<NAME>_FILE` names. An editor can validate an env file against [archiver.schema.json](archiver.schema.json).")
	for _, g := range Groups {
		w("")
		w("## %s", g)
		w("")
		w("| Variable | Default | Description |")
		w("|---|---|---|")
		for _, s := range Reference {
			if s.Group == g {
				w("| %s | %s | %s |", cell(s), dflt(s), desc(s))
			}
		}
	}
	w("")
	w("## Storage targets")
	w("")
	w("Each storage is `STORAGE_TARGET_<N>_*`, numbered from 1 without gaps; target 1 is the primary, the others are kept as copies of it. The variables a type uses (its fields) are listed with the types that use them.")
	w("")
	w("| Variable | Types | Default | Description |")
	w("|---|---|---|---|")
	for _, s := range TargetReference() {
		types := s.Group
		if types == "" {
			types = "all"
		}
		w("| %s | %s | %s | %s |", cell(Var{Name: "STORAGE_TARGET_<N>_" + s.Name, Secret: s.Secret}), types, dflt(s), desc(s))
	}
	return b.String()
}

func cell(s Var) string {
	if s.Secret {
		return "`" + s.Name + "` (secret)"
	}
	return "`" + s.Name + "`"
}

func dflt(s Var) string {
	if s.Default == "" {
		return ""
	}
	return "`" + s.Default + "`"
}

func desc(s Var) string {
	d := strings.ReplaceAll(s.Desc, "|", "\\|")
	if len(s.Values) > 0 {
		d += " One of `" + strings.Join(s.Values, "`, `") + "`."
	}
	return d
}

// intervalPattern is what ParseInterval accepts: whole days, or a Go duration (either micro
// sign), never zero. A duration too small to count (0.0000000001s) is zero to the parser, and
// past what a pattern can tell.
const intervalPattern = `^\+?(?![0.]+(ns|us|µs|μs|ms|s|m|h|d)([0.]+(ns|us|µs|μs|ms|s|m|h))*$)([0-9]+d|(([0-9]+\.?[0-9]*|\.[0-9]+)(ns|us|µs|μs|ms|s|m|h))+)$`

// kindPattern is what a value of a kind must look like, for the schema (ECMA-262 regular
// expressions, as JSON Schema uses).
var kindPattern = map[string]string{
	"integer":  `^[1-9][0-9]*$`,
	"boolean":  `^([Tt][Rr][Uu][Ee]|[Ff][Aa][Ll][Ss][Ee])$`,
	"port":     `^([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$`,
	"interval": intervalPattern,
	"url":      `^https?://`,
}

// ReferenceSchema is docs/archiver.schema.json: a JSON Schema of an env file's variables,
// so an editor completes and checks them (ADR 44). A secret set as a variable is flagged:
// it belongs in a file.
func ReferenceSchema() string {
	props := map[string]any{}
	add := func(name string, s Var) {
		if s.Secret {
			props[name] = map[string]any{"description": "Secret: put it in a file (" + strings.ToLower(name) + " in the secrets directory, or name the file in " + name + "_FILE), never an environment variable.", "not": map[string]any{}}
			props[name+"_FILE"] = map[string]any{"type": "string", "description": "A file holding " + name + "."}
			return
		}
		p := valueSchema(s)
		if s.Default != "" {
			p["default"] = s.Default
		}
		props[name] = p
	}
	for _, s := range Reference {
		add(s.Name, s)
	}
	patterns := map[string]any{}
	for _, s := range TargetReference() {
		key := "^STORAGE_TARGET_[0-9]+_" + regexp.QuoteMeta(s.Name) + "$"
		if s.Secret {
			patterns[key] = map[string]any{"description": "Secret: put it in a file, never an environment variable.", "not": map[string]any{}}
			patterns["^STORAGE_TARGET_[0-9]+_"+regexp.QuoteMeta(s.Name)+"_FILE$"] = map[string]any{"type": "string"}
			continue
		}
		patterns[key] = valueSchema(s)
	}
	schema := map[string]any{
		"$schema":           "https://json-schema.org/draft/2020-12/schema",
		"$id":               "https://github.com/SisyphusMD/archiver/blob/main/docs/archiver.schema.json",
		"title":             "Archiver environment",
		"description":       "Archiver's settings as environment variables (an env file or compose environment). Generated from internal/config/reference.go.",
		"type":              "object",
		"properties":        props,
		"patternProperties": patterns,
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	return string(b) + "\n"
}

// valueSchema is what a setting's value may be. Empty is always allowed: the loader takes an
// empty variable as unset. Fixed values are an enum an editor offers; where the loader folds
// case, a pattern also takes them in any letter case.
func valueSchema(s Var) map[string]any {
	p := map[string]any{"type": "string", "description": desc(s)}
	switch re, ok := kindPattern[s.Kind]; {
	case s.Pattern != "":
		p["pattern"] = s.Pattern
	case len(s.Values) > 0 && !s.CaseFold:
		p["enum"] = append([]string{""}, s.Values...)
	case len(s.Values) > 0:
		alts := make([]string, len(s.Values))
		for i, v := range s.Values {
			alts[i] = caseless(v)
		}
		p["anyOf"] = []any{
			map[string]any{"enum": append([]string{""}, s.Values...)},
			map[string]any{"pattern": "^(" + strings.Join(alts, "|") + ")$"},
		}
	case ok:
		p["pattern"] = "^$|" + re
	}
	return p
}

// caseless is a pattern matching v in any letter case, spelled out: schema patterns follow
// ECMA-262, which has no (?i) flag.
func caseless(v string) string {
	var b strings.Builder
	for _, r := range v {
		lo, up := strings.ToLower(string(r)), strings.ToUpper(string(r))
		if lo != up {
			b.WriteString("[" + up + lo + "]")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}
