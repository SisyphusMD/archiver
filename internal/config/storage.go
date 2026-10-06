package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Sanitize turns a storage name into the Duplicacy storage name and the fragment of its
// DUPLICACY_<NAME>_* variables. It works on bytes, so a multi-byte character becomes one
// underscore per byte; a leading digit gets an underscore in front. Existing storages were
// created under these names, so this must never change.
func Sanitize(name string) string {
	b := []byte(name)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
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
	switch t.Type {
	case "local":
		return t.LocalPath, nil
	case "sftp":
		return fmt.Sprintf("sftp://%s@%s:%s//%s", t.SFTPUser, t.SFTPURL, t.SFTPPort, t.SFTPPath), nil
	case "b2":
		return "b2://" + t.B2Bucket, nil
	case "s3":
		region := t.S3Region
		if region == "" {
			region = "none"
		}
		return fmt.Sprintf("s3://%s@%s/%s", region, t.S3Endpoint, t.S3Bucket), nil
	}
	return "", fmt.Errorf("%s is not a supported backup type (local, sftp, b2, s3)", t.Type)
}

// DuplicacyEnv is the credentials Duplicacy needs for this target, as NAME=value pairs.
// Environment variables are the only way they reach Duplicacy: `duplicacy set -value`
// would store them in plain text inside the backed-up data.
func (c *Config) DuplicacyEnv(t Target, sshKeyFile string) []string {
	p := t.EnvPrefix()
	env := []string{
		p + "PASSWORD=" + c.StoragePassword,
		p + "RSA_PASSPHRASE=" + c.RSAPassphrase,
	}
	switch t.Type {
	case "sftp":
		env = append(env, p+"SSH_KEY_FILE="+sshKeyFile)
	case "b2":
		env = append(env, p+"B2_ID="+t.B2ID, p+"B2_KEY="+t.B2Key)
	case "s3":
		env = append(env, p+"S3_ID="+t.S3ID, p+"S3_SECRET="+t.S3Secret)
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
