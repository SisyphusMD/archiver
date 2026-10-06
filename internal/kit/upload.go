package kit

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
)

// remotes describe, per storage type, the rclone remote that reaches it (ADR 24): its
// RCLONE_CONFIG_KIT_* settings, and the directory on it where the kit goes, beside the
// backups. Credentials only ever travel in this environment, never argv or disk.
var remotes = map[string]func(r *Run, t config.Target) (settings map[string]string, dir string, err error){
	"local": func(r *Run, t config.Target) (map[string]string, string, error) {
		return map[string]string{"TYPE": "local"}, t.LocalPath, nil
	},
	"sftp": func(r *Run, t config.Target) (map[string]string, string, error) {
		known, algorithms, err := r.knownHosts(t)
		if err != nil {
			return nil, "", err
		}
		// No shell commands on the server (many allow SFTP only): rclone then cannot hash
		// remotely and checks the size, as the sftp put it replaces did.
		return map[string]string{
			"TYPE": "sftp", "HOST": t.SFTPURL, "PORT": t.SFTPPort, "USER": t.SFTPUser,
			"KEY_FILE": r.Layout.SSHPrivateKey(), "KNOWN_HOSTS_FILE": known, "SHELL_TYPE": "none",
			// A server may forbid SETSTAT; the kit needs no timestamp, and the mode step after
			// tolerates a refused chmod.
			"SET_MODTIME": "false",
			// Only the key types already trusted for this host: offered another type the
			// server also has, rclone would call it a changed key and refuse.
			"HOST_KEY_ALGORITHMS": algorithms,
		}, "/" + t.SFTPPath, nil
	},
	"b2": func(r *Run, t config.Target) (map[string]string, string, error) {
		return map[string]string{"TYPE": "b2", "ACCOUNT": t.B2ID, "KEY": t.B2Key}, t.B2Bucket, nil
	},
	"s3": func(r *Run, t config.Target) (map[string]string, string, error) {
		region := t.S3Region
		// duplicacy's "none" (region-less endpoints such as MinIO); SigV4 needs some region.
		if region == "" || region == "none" {
			region = "us-east-1"
		}
		return map[string]string{
			"TYPE": "s3", "PROVIDER": "Other", "ACCESS_KEY_ID": t.S3ID, "SECRET_ACCESS_KEY": t.S3Secret,
			"ENDPOINT": "https://" + t.S3Endpoint, "REGION": region, "FORCE_PATH_STYLE": "true",
			// A key limited to its bucket may not create or list buckets.
			"NO_CHECK_BUCKET": "true",
		}, t.S3Bucket, nil
	},
}

// after are the steps some types take once the kit is placed: making it as readable as the
// storage's own files, where the storage has file modes.
var after = map[string]func(r *Run, t config.Target, names []string) int{
	"local": (*Run).localAccess,
	"sftp":  (*Run).sftpAccess,
}

func (r *Run) rclone() string {
	if r.Rclone != "" {
		return r.Rclone
	}
	return "rclone"
}

// upload places the kit and its README on t, returning OK, Failed or Unverified.
func (r *Run) upload(t config.Target, kit, readme string) int {
	remote, ok := remotes[t.Type]
	if !ok {
		return Failed
	}
	settings, dir, err := remote(r, t)
	if err != nil {
		r.warning(fmt.Sprintf("Recovery kit: cannot reach storage '%s': %v", t.Name, err))
		return Failed
	}
	env := append(os.Environ(), "RCLONE_CONFIG_KIT_TYPE="+settings["TYPE"])
	for k, v := range settings {
		if k != "TYPE" {
			env = append(env, "RCLONE_CONFIG_KIT_"+k+"="+v)
		}
	}
	dest := "KIT:" + strings.TrimSuffix(dir, "/") + "/"
	// The kit belongs beside a storage, whose root always holds Duplicacy's config: rclone
	// would otherwise create a missing directory (an unmounted path, a mistyped bucket) and
	// report success.
	stat := exec.Command(r.rclone(), "--config", os.DevNull, "lsjson", "--stat", "--contimeout", "15s", "--timeout", "5m", dest+"config")
	stat.Env = env
	if out, err := stat.CombinedOutput(); err != nil {
		r.warning(fmt.Sprintf("Recovery kit: storage '%s' has no Duplicacy config at %s, so the kit is not placed there: %s", t.Name, dir, strings.TrimSpace(string(out))))
		return Failed
	}
	names := []string{r.KitName(), r.readmeName()}
	for i, src := range []string{kit, readme} {
		// rclone writes a temporary file and renames it over the live one once it is complete
		// and checked, so a failed upload leaves the previous kit in place. --ignore-times: a
		// kit chosen for upload is always sent; matching size and time (coarse on some servers)
		// would otherwise skip a changed kit and leave the old one marked current.
		cmd := exec.Command(r.rclone(), "--config", os.DevNull, "copyto", "--ignore-times", "--retries", "3",
			"--contimeout", "15s", "--timeout", "5m", "-q", src, dest+names[i])
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			r.warning(fmt.Sprintf("Recovery kit: upload of %s to storage '%s' failed: %s", names[i], t.Name, strings.TrimSpace(string(out))))
			return Failed
		}
	}
	if step, ok := after[t.Type]; ok {
		return step(r, t, names)
	}
	return OK
}

// modeGrants reports whether mode have grants every group/other read bit want grants: the
// access a mirror or backup user copies the store with. An owner-only kit shuts it out of the
// one file it needs in a disaster; a more permissive one passes.
func modeGrants(have, want os.FileMode) bool { return want&0o044&^have == 0 }

// chmod is os.Chmod; tests stand in a share that ignores it.
var chmod = os.Chmod

// localAccess gives the placed files the owner and mode of the storage's 'config' (the
// storage's own access model; the kit is encrypted already) and reports Unverified when one
// ends up less readable. rclone's rename leaves a fresh inode, which inherits the directory's
// ACLs; a chmod is made only when it changes the mode, because on an ACL-backed share
// (Synology's) any chmod discards the inherited ACL, even one to the mode already shown.
func (r *Run) localAccess(t config.Target, names []string) int {
	ref, err := os.Stat(filepath.Join(t.LocalPath, "config"))
	if err != nil {
		return OK
	}
	status := OK
	for _, n := range names {
		p := filepath.Join(t.LocalPath, n)
		if st, ok := ref.Sys().(*syscall.Stat_t); ok {
			os.Chown(p, int(st.Uid), int(st.Gid))
		}
		fi, err := os.Stat(p)
		if err == nil && fi.Mode().Perm() != ref.Mode().Perm() {
			chmod(p, ref.Mode().Perm())
			fi, err = os.Stat(p)
		}
		if err != nil || !modeGrants(fi.Mode().Perm(), ref.Mode().Perm()) {
			status = Unverified
		}
	}
	if status != OK {
		r.warning(fmt.Sprintf("Recovery kit in '%s' is less readable than its config; retrying on the next run.", t.LocalPath))
	}
	return status
}

var lsPerms = regexp.MustCompile(`(?m)^[-dbclps]([-rwxsStT]{9})`)

// symbolicMode converts ls-style permissions (rwxr-xr-x) to a mode, dropping the special bits.
func symbolicMode(perms string) os.FileMode {
	var m os.FileMode
	for i := 0; i < 9; i += 3 {
		var d os.FileMode
		if perms[i] == 'r' {
			d |= 4
		}
		if perms[i+1] == 'w' {
			d |= 2
		}
		switch perms[i+2] {
		case 'x', 's', 't':
			d |= 1
		}
		m = m<<3 | d
	}
	return m
}

// knownHosts is the host-key file for t's server and the key algorithms trusted for it,
// adding the server's keys the first time (trust on first use, as ssh's accept-new: a later
// change of key fails the connection).
func (r *Run) knownHosts(t config.Target) (path, algorithms string, err error) {
	dir := filepath.Join(os.Getenv("HOME"), ".ssh")
	if dir == ".ssh" {
		dir = "/root/.ssh"
	}
	path = filepath.Join(dir, "known_hosts")
	host := t.SFTPURL
	if t.SFTPPort != "" && t.SFTPPort != "22" {
		host = "[" + t.SFTPURL + "]:" + t.SFTPPort
	}
	if algorithms = trusted(path, host); algorithms != "" {
		return path, algorithms, nil
	}
	port := t.SFTPPort
	if port == "" {
		port = "22"
	}
	keys, err := exec.Command("ssh-keyscan", "-T", "15", "-p", port, t.SFTPURL).Output()
	if err != nil || len(bytes.TrimSpace(keys)) == 0 {
		return "", "", fmt.Errorf("no host key from %s:%s", t.SFTPURL, port)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return "", "", err
	}
	_, err = f.Write(keys)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", err
	}
	if algorithms = trusted(path, host); algorithms == "" {
		return "", "", fmt.Errorf("no usable host key recorded for %s", host)
	}
	return path, algorithms, nil
}

// trusted lists, space-separated, the host-key algorithms the known-hosts file trusts for
// host; an RSA key allows its SHA-2 signatures too.
func trusted(path, host string) string {
	out, err := exec.Command("ssh-keygen", "-F", host, "-f", path).Output()
	if err != nil {
		return ""
	}
	var algs []string
	add := func(a ...string) {
		for _, x := range a {
			if !slices.Contains(algs, x) {
				algs = append(algs, x)
			}
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[1] == "ssh-rsa" {
			add("rsa-sha2-512", "rsa-sha2-256", "ssh-rsa")
		} else {
			add(f[1])
		}
	}
	return strings.Join(algs, " ")
}

func (r *Run) sftp(t config.Target, batch string) ([]byte, error) {
	known, _, err := r.knownHosts(t)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("sftp", "-q", "-P", t.SFTPPort, "-i", r.Layout.SSHPrivateKey(),
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+known,
		"-b", "-", t.SFTPUser+"@"+t.SFTPURL)
	cmd.Stdin = bytes.NewBufferString(batch)
	return cmd.Output()
}

// sftpMode reads a remote file's mode; ok is false when none came back (an object-store
// gateway may report none).
func (r *Run) sftpMode(t config.Target, path string) (os.FileMode, bool) {
	out, _ := r.sftp(t, "ls -l "+path+"\n")
	m := lsPerms.FindSubmatch(out)
	if m == nil {
		return 0, false
	}
	mode := symbolicMode(string(m[1]))
	return mode, mode != 0
}

// sftpAccess stamps the placed files with the mode the storage's own files carry (else 644,
// readable by a mirror user; never an owner-only umask), then reads it back: a server that
// ignored the chmod still reports success, and recording that would freeze an owner-only
// kit in place. An unreadable mode is not a pass.
func (r *Run) sftpAccess(t config.Target, names []string) int {
	dir := "/" + t.SFTPPath
	mode, ok := r.sftpMode(t, dir+"/config")
	if !ok {
		mode = 0o644
	}
	var batch strings.Builder
	for _, n := range names {
		// Best effort ('-'): a server may forbid SETSTAT.
		fmt.Fprintf(&batch, "-chmod %o %s/%s\n", mode, dir, n)
	}
	r.sftp(t, batch.String())
	target := t.SFTPUser + "@" + t.SFTPURL
	placed, ok := r.sftpMode(t, dir+"/"+names[0])
	if !ok {
		r.warning(fmt.Sprintf("Recovery kit uploaded to '%s' but its mode could not be read back; re-placing on the next run.", target))
		return Unverified
	}
	if !modeGrants(placed, mode) {
		r.warning(fmt.Sprintf("Recovery kit on '%s' is mode %o, less readable than %o; re-placing on the next run.", target, placed, mode))
		return Unverified
	}
	return OK
}
