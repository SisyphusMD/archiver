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

// remote is the rclone remote that reaches t for the kit (ADR 24): the type's settings
// (config.Types), plus what needs local state: the SSH key and host keys for SFTP, rclone's
// obscured form of a password, a Google Drive token file's contents, OneDrive's writable
// token. Credentials only ever travel in this environment, never argv or disk.
func (r *Run) remote(t config.Target) (map[string]string, string, error) {
	typ, ok := config.Types[t.Type]
	if !ok || typ.Remote == nil {
		return nil, "", fmt.Errorf("no recovery-kit remote for storage type %s", t.Type)
	}
	settings, dir := typ.Remote(t.Values)
	switch t.Type {
	case "sftp", "sftpc":
		known, algorithms, err := r.knownHosts(t)
		if err != nil {
			return nil, "", err
		}
		// No shell commands on the server (many allow SFTP only): rclone then cannot hash
		// remotely and checks the size, as the sftp put it replaces did. A server may forbid
		// SETSTAT; the kit needs no timestamp, and the mode step after tolerates a refused
		// chmod. Only the key types already trusted for this host are offered: offered another
		// type the server also has, rclone would call it a changed key and refuse.
		for k, v := range map[string]string{"KEY_FILE": r.Layout.SSHPrivateKey(), "KNOWN_HOSTS_FILE": known,
			"SHELL_TYPE": "none", "SET_MODTIME": "false", "HOST_KEY_ALGORITHMS": algorithms} {
			settings[k] = v
		}
	case "webdav", "webdav-http", "smb":
		obscured, err := r.obscure(settings["PASS"])
		if err != nil {
			return nil, "", err
		}
		settings["PASS"] = obscured
	case "one", "odb":
		pre := strings.ToUpper(t.Type) + "_"
		for _, f := range typ.Fields {
			if !f.Rotates {
				continue
			}
			token, id, driveType, err := oneDrive(config.WritableToken(t, f), t.Get(pre+"CLIENT_ID"), t.Get(pre+"CLIENT_SECRET"), t.Get(pre+"DRIVE_ID"))
			if err != nil {
				return nil, "", err
			}
			settings["TOKEN"], settings["DRIVE_ID"], settings["DRIVE_TYPE"] = token, id, driveType
		}
	}
	return settings, dir, nil
}

// obscure is rclone's reversible encoding of a password, which its config expects; the
// password reaches rclone on stdin.
func (r *Run) obscure(pass string) (string, error) {
	cmd := exec.Command(r.rclone(), "obscure", "-")
	cmd.Stdin = strings.NewReader(pass)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rclone obscure: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// after are the steps some types take once the kit is placed: making it as readable as the
// storage's own files, where the storage has file modes.
var after = map[string]func(r *Run, t config.Target, names []string) int{
	"local": (*Run).localAccess,
	"sftp":  (*Run).sftpAccess,
	"sftpc": (*Run).sftpAccess,
}

func (r *Run) rclone() string {
	if r.Rclone != "" {
		return r.Rclone
	}
	return "rclone"
}

// upload places the kit and its README on t, returning OK, Failed or Unverified.
func (r *Run) upload(t config.Target, kit, readme string) int {
	settings, dir, err := r.remote(t)
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

// chmod is (*os.File).Chmod; tests stand in a share that ignores it.
var chmod = (*os.File).Chmod

// localAccess gives the placed files the owner and mode of the storage's 'config' (the
// storage's own access model; the kit is encrypted already) and reports Unverified when one
// ends up less readable. rclone's rename leaves a fresh inode, which inherits the directory's
// ACLs; a chmod is made only when it changes the mode, because on an ACL-backed share
// (Synology's) any chmod discards the inherited ACL, even one to the mode already shown.
//
// Whoever owns the storage directory can swap entries in it, so nothing here follows a
// link: a symlink put where the kit was would otherwise hand them ownership of whatever it
// points at (the secrets, the keys).
func (r *Run) localAccess(t config.Target, names []string) int {
	ref, err := os.Lstat(filepath.Join(t.Get("LOCAL_PATH"), "config"))
	if err != nil || !ref.Mode().IsRegular() {
		return OK
	}
	status := OK
	for _, n := range names {
		if !r.adjust(filepath.Join(t.Get("LOCAL_PATH"), n), ref) {
			status = Unverified
		}
	}
	if status != OK {
		r.warning(fmt.Sprintf("Recovery kit in '%s' is less readable than its config; retrying on the next run.", t.Get("LOCAL_PATH")))
	}
	return status
}

// adjust sets one placed file's owner and mode through a descriptor opened without
// following links, and reports whether it is as readable as ref.
func (r *Run) adjust(p string, ref os.FileInfo) bool {
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	if st, ok := ref.Sys().(*syscall.Stat_t); ok {
		f.Chown(int(st.Uid), int(st.Gid))
	}
	if fi.Mode().Perm() != ref.Mode().Perm() {
		chmod(f, ref.Mode().Perm())
		if fi, err = f.Stat(); err != nil {
			return false
		}
	}
	return modeGrants(fi.Mode().Perm(), ref.Mode().Perm())
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
	host := t.Get("SFTP_URL")
	if t.Get("SFTP_PORT") != "" && t.Get("SFTP_PORT") != "22" {
		host = "[" + t.Get("SFTP_URL") + "]:" + t.Get("SFTP_PORT")
	}
	if algorithms = trusted(path, host); algorithms != "" {
		return path, algorithms, nil
	}
	port := t.Get("SFTP_PORT")
	if port == "" {
		port = "22"
	}
	keys, err := exec.Command("ssh-keyscan", "-T", "15", "-p", port, t.Get("SFTP_URL")).Output()
	if err != nil || len(bytes.TrimSpace(keys)) == 0 {
		return "", "", fmt.Errorf("no host key from %s:%s", t.Get("SFTP_URL"), port)
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
	cmd := exec.Command("sftp", "-q", "-P", t.Get("SFTP_PORT"), "-i", r.Layout.SSHPrivateKey(),
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+known,
		"-b", "-", t.Get("SFTP_USER")+"@"+t.Get("SFTP_URL"))
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
	dir := "/" + t.Get("SFTP_PATH")
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
	target := t.Get("SFTP_USER") + "@" + t.Get("SFTP_URL")
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
