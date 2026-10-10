package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The parsers of what a person configures (ADR 39): none may panic on any input, and each
// keeps its own rule whatever it is given.

func FuzzSplitServiceDirectories(f *testing.F) {
	for _, s := range []string{"/srv/*/", "/a:/b\n/c", "::\n\n", `/x\:y`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		for _, d := range splitServiceDirectories(raw) {
			if d == "" {
				t.Fatalf("an empty entry from %q", raw)
			}
		}
	})
}

func FuzzParseInterval(f *testing.F) {
	// 110000d wrapped negative before durations were bounded (found by this fuzzer).
	for _, s := range []string{"7d", "12h", "90m", "0", "-1d", "1e9h", "d", "110000d"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if d, err := ParseInterval(s); err == nil && s != "" && d <= 0 {
			t.Fatalf("%q parsed to %v", s, d)
		}
	})
}

func FuzzMatch(f *testing.F) {
	// "[[:]" panicked (overlapping [: and :], found by this fuzzer).
	for _, p := range [][2]string{{"*", "app"}, {"app[!2]", "app1"}, {`a\*b`, "a*b"}, {"[[:alpha:]]*", "x"}, {"[", "["}, {`\`, `\`}, {"[[:]", "0"}} {
		f.Add(p[0], p[1])
	}
	f.Fuzz(func(t *testing.T, pattern, name string) {
		match(pattern, name)
		unescape(pattern)
	})
}

// Storage names map to Duplicacy storage names and DUPLICACY_<NAME>_* variables: the
// mapping yields only letters, digits and underscores, and is stable.
func FuzzSanitize(f *testing.F) {
	for _, s := range []string{"offsite", "my-b2", "ünïcode", "", "a b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		s := Sanitize(name)
		for _, c := range []byte(s) {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
				t.Fatalf("%q -> %q", name, s)
			}
		}
		if Sanitize(s) != s {
			t.Fatalf("not stable: %q -> %q -> %q", name, s, Sanitize(s))
		}
	})
}

// A whole configuration from an environment and a secret file, then every storage URL.
func FuzzLoad(f *testing.F) {
	f.Add("STORAGE_TARGET_1_NAME=local\nSTORAGE_TARGET_1_TYPE=local\nSTORAGE_TARGET_1_LOCAL_PATH=/b\nSERVICE_DIRECTORIES=/srv/*/", "password1")
	f.Add("STORAGE_TARGET_1_NAME=x\nSTORAGE_TARGET_1_TYPE=sftp\nSTORAGE_TARGET_1_SFTP_URL=h\nSTORAGE_TARGET_1_SFTP_PORT=x", "\r\n")
	f.Add("STORAGE_TARGET_1_TYPE=s3\nSTORAGE_TARGET_1_S3_ENDPOINT=e:1\nNOTIFY_ON=everything\nMETRICS_PORT=0", "")
	f.Fuzz(func(t *testing.T, env, secret string) {
		vals := map[string]string{}
		for _, line := range strings.Split(env, "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				vals[k] = v
			}
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "storage_password"), []byte(secret), 0o600); err != nil {
			t.Fatal(err)
		}
		src := Source{Getenv: func(k string) string { return vals[k] }, SecretsDir: dir}
		environ := make([]string, 0, len(vals))
		for k, v := range vals {
			environ = append(environ, k+"="+v)
		}
		sort.Strings(environ)
		cfg, _, err := Load(src, environ)
		if err != nil {
			return
		}
		_ = cfg.Validate(dir)
		for _, tg := range cfg.Targets {
			_, _ = tg.URL()
			_ = tg.StorageName()
		}
	})
}
