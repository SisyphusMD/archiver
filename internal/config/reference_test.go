package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every setting and secret the code accepts is in the reference (ADR 43): the settings and
// secrets the loader recognizes are read from its own patterns.
func TestReferenceComplete(t *testing.T) {
	documented := map[string]bool{}
	for _, s := range Reference {
		if documented[s.Name] {
			t.Errorf("%s listed twice", s.Name)
		}
		documented[s.Name] = true
		if s.Desc == "" || !contains(Groups, s.Group) {
			t.Errorf("%s needs a description and a known group", s.Name)
		}
	}
	for _, re := range []*regexp.Regexp{globalSetting, globalSecret} {
		inner := strings.TrimSuffix(strings.TrimPrefix(re.String(), "^("), ")$")
		for _, name := range strings.Split(inner, "|") {
			if !documented[name] {
				t.Errorf("%s is accepted but missing from Reference", name)
			}
		}
	}
	for _, s := range TargetReference() {
		if s.Desc == "" {
			t.Errorf("STORAGE_TARGET_<N>_%s has no description (its field's Prompt)", s.Name)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// docs/configuration.md and docs/archiver.schema.json are what Reference generates; CI fails
// when they are stale. UPDATE_REFERENCE=1 rewrites them.
func TestGeneratedReferenceCurrent(t *testing.T) {
	for file, want := range map[string]string{
		"configuration.md":     ReferenceMarkdown(),
		"archiver.schema.json": ReferenceSchema(),
	} {
		path := filepath.Join("..", "..", "docs", file)
		if os.Getenv("UPDATE_REFERENCE") != "" {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("docs/%s is out of date: run UPDATE_REFERENCE=1 go test ./internal/config", file)
		}
	}
}

// notSettings are variables the code reads that a person does not configure: the process's
// own, the recovery kit step's from its parent, and old names read only to refuse them.
var notSettings = map[string]bool{
	"HOME": true, "PATH": true, "ARCHIVER_KIT_PRIMARY_MARKER": true, "ARCHIVER_KIT_SKIP_TARGETS": true,
	"CRON_SCHEDULE": true, "BUNDLE_PASSWORD": true,
}

// Every variable the code reads by name is documented: the whole tree is scanned for
// Getenv("NAME") and getenv("NAME").
func TestEveryReadVariableDocumented(t *testing.T) {
	documented := map[string]bool{}
	for _, s := range Reference {
		documented[s.Name] = true
	}
	read := regexp.MustCompile(`(?:Getenv|getenv)\("([A-Z][A-Z0-9_]+)"\)`)
	for _, root := range []string{"../../internal", "../../cmd"} {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, _ := os.ReadFile(p)
			for _, m := range read.FindAllStringSubmatch(string(b), -1) {
				if !documented[m[1]] && !notSettings[m[1]] {
					t.Errorf("%s reads %s, which is not in Reference", p, m[1])
				}
			}
			return nil
		})
	}
}
