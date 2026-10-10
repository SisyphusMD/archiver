package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sanitize turns a storage name into the Duplicacy storage name and the fragment of its
// DUPLICACY_<NAME>_* variables. It works on bytes, so a multi-byte character becomes one
// underscore per byte; a leading digit gets an underscore in front. Existing storages were
// created under these names, so this must never change.
func Sanitize(name string) string {
	b := []byte(name)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') { //nolint:staticcheck // kept exactly as written: storage names never change
			b[i] = '_'
		}
	}
	s := string(b)
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	return s
}

// StorageName is the target's Duplicacy storage name.
func (t Target) StorageName() string { return Sanitize(t.Name) }

// EnvPrefix is the prefix of the variables Duplicacy reads this storage's credentials
// from. Duplicacy reads a storage named exactly "default" without the name.
func (t Target) EnvPrefix() string {
	s := t.StorageName()
	if s == "default" {
		return "DUPLICACY_"
	}
	return "DUPLICACY_" + strings.ToUpper(s) + "_"
}

// URL is the Duplicacy storage URL.
func (t Target) URL() (string, error) {
	typ, ok := Types[t.Type]
	if !ok {
		return "", fmt.Errorf("%s is not a supported storage type (%s)", t.Type, strings.Join(StorageTypes, ", "))
	}
	return typ.URL(t.withDefaults()), nil
}

// withDefaults is the target's values with each empty field at its type's default.
func (t Target) withDefaults() Values {
	v := Values{}
	for k, x := range t.Values {
		v[k] = x
	}
	for _, f := range Types[t.Type].Fields {
		if v[f.Name] == "" {
			v[f.Name] = f.Default
		}
	}
	return v
}

// TokenDir holds writable copies of the token files Duplicacy rewrites as it refreshes them
// (OneDrive): the secrets themselves are mounted read-only.
var TokenDir = "/opt/archiver/logs/.tokens" //nolint:gosec // G101: a path or public endpoint, not a credential

// DuplicacyEnv is the credentials Duplicacy needs for this target, as NAME=value pairs.
// Environment variables are the only way they reach Duplicacy: `duplicacy set -value`
// would store them in plain text inside the backed-up data.
func (c *Config) DuplicacyEnv(t Target, sshKeyFile string) []string {
	p := t.EnvPrefix()
	env := []string{
		p + "PASSWORD=" + c.StoragePassword,
		p + "RSA_PASSPHRASE=" + c.RSAPassphrase,
	}
	if t.Type == "sftp" || t.Type == "sftpc" {
		env = append(env, p+"SSH_KEY_FILE="+sshKeyFile)
	}
	for _, f := range Types[t.Type].Fields {
		if f.Key == "" || t.Values[f.Name] == "" {
			continue
		}
		v := t.Values[f.Name]
		if f.Path {
			v = t.Values[f.Name+"_FILE"]
			if f.Rotates {
				v = WritableToken(t, f)
			}
		}
		env = append(env, p+strings.ToUpper(f.Key)+"="+v)
	}
	return env
}

// DuplicacyEnviron is environ without raw secrets, plus every target's credentials: the
// environment every duplicacy command runs with.
func (c *Config) DuplicacyEnviron(environ []string, sshKeyFile string) []string {
	var env []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !IsSecret(name) {
			env = append(env, kv)
		}
	}
	for _, t := range c.Targets {
		env = append(env, c.DuplicacyEnv(t, sshKeyFile)...)
	}
	return env
}

// StorageFingerprint identifies the configured storages (names and URLs, in order), so a
// daemon's copy workers can tell whether a backup copies to the storages they keep.
func (c *Config) StorageFingerprint() string {
	h := sha256.New()
	for _, t := range c.Targets {
		url, _ := t.URL()
		fmt.Fprintf(h, "%s=%s\n", t.StorageName(), url)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// WritableToken is the path of t's writable copy of a token secret that Duplicacy rewrites as
// it refreshes it. The copy is seeded from the secret, and seeded again only when the secret
// itself changes (a new token from the user), so a refreshed token survives restarts.
func WritableToken(t Target, f Field) string {
	src, dst := t.Values[f.Name+"_FILE"], tokenCopy(t, f)
	seed, err := os.ReadFile(src)
	if err != nil {
		return src
	}
	if _, err := os.Stat(dst); err == nil && seeded(dst, seed) {
		return dst
	}
	if WritePrivate(dst, seed) != nil || WritePrivate(dst+".seed", []byte(seedMark(seed))) != nil {
		return src
	}
	return dst
}

// WritePrivate replaces path with a new 0600 file holding data. A new file is renamed over
// the old entry, so nothing already at path (a symlink, a readable file someone with access
// to the logs volume put there) receives the token; a directory others can write is refused,
// since they could swap the entry back.
func WritePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() || fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is not a directory only its owner can write", dir)
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// tokenCopy is where t keeps its writable copy of the token field f.
func tokenCopy(t Target, f Field) string {
	return filepath.Join(TokenDir, t.StorageName()+"-"+strings.ToLower(f.Name))
}

// currentToken is the token as Duplicacy last refreshed it into the copy at path, or secret
// when the copy is missing or was seeded from a different secret.
func currentToken(path string, secret []byte) []byte {
	if b, err := os.ReadFile(path); err == nil && seeded(path, secret) {
		return b
	}
	return secret
}

func seeded(path string, secret []byte) bool {
	mark, err := os.ReadFile(path + ".seed")
	return err == nil && string(mark) == seedMark(secret)
}

// seedMark identifies the secret a copy was seeded from, read as secrets are (a trailing
// newline does not count), so the raw file and the value read from it match.
func seedMark(secret []byte) string {
	sum := sha256.Sum256([]byte(strings.TrimSuffix(strings.TrimRight(string(secret), "\n"), "\r")))
	return hex.EncodeToString(sum[:])
}
