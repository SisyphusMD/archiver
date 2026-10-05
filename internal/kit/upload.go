package kit

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
)

func (r *Run) upload(t config.Target, kit, readme string) int {
	switch t.Type {
	case "local":
		return r.uploadLocal(t, kit, readme)
	case "sftp":
		return r.uploadSFTP(t, kit, readme)
	case "b2":
		return r.uploadB2(t, kit, readme)
	case "s3":
		return r.uploadS3(t, kit, readme)
	}
	return Failed
}

// modeGrants reports whether mode have grants every group/other read bit want grants: the
// access a mirror or backup user copies the store with. An owner-only kit shuts it out of the
// one file it needs in a disaster; a more permissive one passes.
func modeGrants(have, want os.FileMode) bool { return want&0o044&^have == 0 }

func (r *Run) uploadLocal(t config.Target, kit, readme string) int {
	ref := filepath.Join(t.LocalPath, "config")
	status := OK
	for _, f := range []struct{ src, name string }{{kit, r.KitName()}, {readme, r.readmeName()}} {
		switch placeLocal(f.src, filepath.Join(t.LocalPath, f.name), ref) {
		case Failed:
			return Failed
		case Unverified:
			status = Unverified
		}
	}
	if status != OK {
		r.warning(fmt.Sprintf("Recovery kit in '%s' is less readable than %s; retrying on the next run.", t.LocalPath, ref))
	}
	return status
}

// placeLocal stages src beside dst, verifies it, then renames it over dst, so the storage is
// never without a kit: a failed or truncated copy leaves the previous one. The staged copy is a
// fresh inode, so it inherits the directory's ACLs, which an in-place overwrite never would.
// It takes ref's owner and mode (the storage's own files' access model, the kit being
// encrypted already), and reports Unverified when it ends up less readable than ref.
func placeLocal(src, dst, ref string) int {
	tmp := fmt.Sprintf("%s.tmp.%d", dst, os.Getpid())
	os.Remove(tmp)
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return Failed
	}
	refInfo, refErr := os.Stat(ref)
	if refErr == nil {
		if st, ok := refInfo.Sys().(*syscall.Stat_t); ok {
			os.Chown(tmp, int(st.Uid), int(st.Gid))
		}
		// Only when it changes the mode: on an ACL-backed share (Synology's) the access that
		// matters is the ACL the fresh inode inherited, invisible from here, and any chmod
		// discards it, even one to the mode already shown.
		if fi, err := os.Stat(tmp); err == nil && fi.Mode().Perm() != refInfo.Mode().Perm() {
			chmod(tmp, refInfo.Mode().Perm())
		}
	}
	// Both hashes must be produced: two failed reads would otherwise compare equal.
	a, errA := hashFile(src)
	b, errB := hashFile(tmp)
	if errA != nil || errB != nil || a != b {
		os.Remove(tmp)
		return Failed
	}
	status := OK
	if refErr == nil {
		fi, err := os.Stat(tmp)
		if err != nil || !modeGrants(fi.Mode().Perm(), refInfo.Mode().Perm()) {
			status = Unverified
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return Failed
	}
	return status
}

// chmod is os.Chmod; tests stand in a share that ignores it.
var chmod = os.Chmod

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

func (r *Run) sftp(t config.Target, batch string) ([]byte, error) {
	cmd := exec.Command("sftp", "-q", "-P", t.SFTPPort, "-i", r.Layout.SSHPrivateKey(),
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-b", "-", t.SFTPUser+"@"+t.SFTPURL)
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

func (r *Run) uploadSFTP(t config.Target, kit, readme string) int {
	// The URL is sftp://user@host:port//path, an absolute path: the kit lands beside the chunks.
	dir := "/" + t.SFTPPath
	target := t.SFTPUser + "@" + t.SFTPURL
	// Stamped with the mode the storage's own files carry, else 644 (readable by a mirror user):
	// never left at an owner-only umask.
	mode, ok := r.sftpMode(t, dir+"/config")
	if !ok {
		mode = 0o644
	}
	kitPath, readmePath := dir+"/"+r.KitName(), dir+"/"+r.readmeName()
	// The puts are strict; the chmods best-effort ('-'), for a server that forbids SETSTAT.
	batch := fmt.Sprintf("put %s %s\nput %s %s\n-chmod %o %s\n-chmod %o %s\n", kit, kitPath, readme, readmePath, mode, kitPath, mode, readmePath)
	if _, err := r.sftp(t, batch); err != nil {
		return Failed
	}
	// Read back: a server that ignored the chmod still reports a clean upload, and recording
	// that would freeze an owner-only kit in place. An unreadable mode is not a pass.
	placed, ok := r.sftpMode(t, kitPath)
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

// curl runs curl with config (credentials, tokens) on stdin, never argv.
func curl(config string, args ...string) ([]byte, error) {
	cmd := exec.Command("curl", append([]string{"-sSf", "--connect-timeout", "15", "--max-time", "300", "-K", "-"}, args...)...)
	cmd.Stdin = bytes.NewBufferString(config)
	return cmd.Output()
}

// curlQuote quotes a value for a curl config file, whose quoted strings know only a few
// backslash escapes.
func curlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s) + `"`
}

// b2API is B2's API; tests point it at a fake.
var b2API = "https://api.backblazeb2.com"

// uploadB2 uses the native B2 API (duplicacy cannot upload arbitrary files).
func (r *Run) uploadB2(t config.Target, kit, readme string) int {
	out, err := curl(fmt.Sprintf("user = %s\nurl = %s\n", curlQuote(t.B2ID+":"+t.B2Key), curlQuote(b2API+"/b2api/v2/b2_authorize_account")),
		"--retry", "3", "--retry-all-errors")
	if err != nil {
		r.warning("B2 authorization failed for recovery-kit upload.")
		return Failed
	}
	var auth struct {
		APIURL    string `json:"apiUrl"`
		Token     string `json:"authorizationToken"`
		AccountID string `json:"accountId"`
		Allowed   struct {
			BucketID   string `json:"bucketId"`
			BucketName string `json:"bucketName"`
		} `json:"allowed"`
	}
	if json.Unmarshal(out, &auth) != nil || auth.APIURL == "" || auth.Token == "" {
		r.warning("B2 authorization failed for recovery-kit upload.")
		return Failed
	}
	header := func(url string) string {
		return fmt.Sprintf("header = %s\nurl = %s\n", curlQuote("Authorization: "+auth.Token), curlQuote(url))
	}
	// A bucket-restricted key names its bucket; an unrestricted one looks it up by name.
	bucketID := ""
	if auth.Allowed.BucketName == t.B2Bucket {
		bucketID = auth.Allowed.BucketID
	}
	if bucketID == "" {
		body, _ := json.Marshal(map[string]string{"accountId": auth.AccountID, "bucketName": t.B2Bucket})
		out, err := curl(header(auth.APIURL+"/b2api/v2/b2_list_buckets"), "--data", string(body))
		if err != nil {
			r.warning("B2 bucket lookup failed for recovery-kit upload.")
			return Failed
		}
		var list struct {
			Buckets []struct {
				BucketID string `json:"bucketId"`
			} `json:"buckets"`
		}
		if json.Unmarshal(out, &list) == nil && len(list.Buckets) > 0 {
			bucketID = list.Buckets[0].BucketID
		}
	}
	if bucketID == "" {
		r.warning(fmt.Sprintf("Could not resolve B2 bucket id for '%s'.", t.B2Bucket))
		return Failed
	}
	for _, f := range []struct{ path, name string }{{kit, r.KitName()}, {readme, r.readmeName()}} {
		body, _ := json.Marshal(map[string]string{"bucketId": bucketID})
		out, err := curl(header(auth.APIURL+"/b2api/v2/b2_get_upload_url"), "--data", string(body))
		var up struct {
			UploadURL string `json:"uploadUrl"`
			Token     string `json:"authorizationToken"`
		}
		if err != nil || json.Unmarshal(out, &up) != nil || up.UploadURL == "" {
			r.warning("B2 get_upload_url failed for recovery-kit upload.")
			return Failed
		}
		sum, err := sha1File(f.path)
		if err != nil {
			return Failed
		}
		cfg := fmt.Sprintf("header = %s\nheader = %s\nheader = \"Content-Type: b2/x-auto\"\nheader = %s\nurl = %s\n",
			curlQuote("Authorization: "+up.Token), curlQuote("X-Bz-File-Name: "+f.name), curlQuote("X-Bz-Content-Sha1: "+sum), curlQuote(up.UploadURL))
		if _, err := curl(cfg, "--data-binary", "@"+f.path, "-o", "/dev/null"); err != nil {
			r.warning(fmt.Sprintf("B2 upload of %s failed.", f.name))
			return Failed
		}
	}
	return OK
}

func sha1File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// uploadS3 uses curl's SigV4 signing with path-style addressing (AWS and MinIO alike).
func (r *Run) uploadS3(t config.Target, kit, readme string) int {
	region := t.S3Region
	// duplicacy's "none" (region-less endpoints such as MinIO); SigV4 needs some region.
	if region == "" || region == "none" {
		region = "us-east-1"
	}
	cfg := fmt.Sprintf("user = %s\naws-sigv4 = %s\n", curlQuote(t.S3ID+":"+t.S3Secret), curlQuote("aws:amz:"+region+":s3"))
	for _, f := range []struct{ path, name string }{{kit, r.KitName()}, {readme, r.readmeName()}} {
		if _, err := curl(cfg, "-T", f.path, "https://"+t.S3Endpoint+"/"+t.S3Bucket+"/"+f.name); err != nil {
			r.warning(fmt.Sprintf("S3 upload of %s failed.", f.name))
			return Failed
		}
	}
	return OK
}
