package main

// Dropbox API v2 as Duplicacy (gilbertchen/go-dropbox, with archiver's dropbox-app.patch) and
// rclone (dropbox-sdk-go-unofficial) call it. Paths name files under one directory; matching is
// case-sensitive, which both clients' consistent spelling never tells apart.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The one app this Dropbox knows, as a real account knows the apps it authorized: a refresh
// token works only with the app it was issued to, so a round trip proves the app's own key and
// secret reached the refresh. backends.d/dropbox.sh configures archiver with these.
const (
	dropboxAppKey       = "archiver-test-app-key"
	dropboxAppSecret    = "archiver-test-app-secret"
	dropboxRefreshToken = "archiver-test-refresh-token"
	dropboxPageSize     = 100
	dropboxHashBlock    = 4 << 20
)

type dropboxFake struct {
	mu       sync.Mutex
	tokens   map[string]bool
	sessions map[string]*dropboxSession
	hashes   map[string]dropboxHash
	// Ids stay with a file or folder through moves, as Dropbox's do; rclone downloads by id.
	ids   map[string]string // path -> id
	paths map[string]string // id -> path
}

type dropboxSession struct {
	file   string
	offset int64
	closed bool
}

type dropboxHash struct {
	size  int64
	mtime time.Time
	hash  string
}

// dropboxErr is an endpoint error: HTTP 409 with the tags of its summary nested as the SDKs
// decode them ("to/conflict/file" -> {".tag":"to","to":{".tag":"conflict",...}}).
type dropboxErr struct {
	summary string
	extra   map[string]any // fields of the innermost union
}

func (e *dropboxErr) Error() string { return e.summary }

func dbxErr(summary string) *dropboxErr { return &dropboxErr{summary: summary} }

func init() {
	register(newDropboxFake(), "api.dropboxapi.com", "content.dropboxapi.com")
}

func newDropboxFake() *dropboxFake {
	return &dropboxFake{tokens: map[string]bool{}, sessions: map[string]*dropboxSession{}, hashes: map[string]dropboxHash{},
		ids: map[string]string{}, paths: map[string]string{}}
}

func (d *dropboxFake) root() string    { return filepath.Join(dataDir, "dropbox", "files") }
func (d *dropboxFake) scratch() string { return filepath.Join(dataDir, "dropbox", "uploads") }
func (d *dropboxFake) local(p string) string {
	return filepath.Join(d.root(), filepath.FromSlash(strings.TrimPrefix(p, "/")))
}

var dropboxContentRoutes = map[string]bool{
	"/2/files/upload": true, "/2/files/download": true,
	"/2/files/upload_session/start": true, "/2/files/upload_session/append_v2": true,
	"/2/files/upload_session/append": true, "/2/files/upload_session/finish": true,
}

func (d *dropboxFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.SplitN(strings.ToLower(r.Host), ":", 2)[0]
	if r.Method != http.MethodPost {
		dropboxPlain(w, http.StatusMethodNotAllowed, "Only POST is supported")
		return
	}
	if r.URL.Path == "/oauth2/token" && host == "api.dropboxapi.com" {
		d.token(w, r)
		return
	}
	content := dropboxContentRoutes[r.URL.Path]
	if content != (host == "content.dropboxapi.com") {
		dropboxPlain(w, http.StatusBadRequest, "Error in call to API function \""+strings.TrimPrefix(r.URL.Path, "/2/")+"\": unknown host for this route")
		return
	}
	if !d.authorized(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error_summary":"invalid_access_token/..","error":{".tag":"invalid_access_token"}}`)
		return
	}

	var arg map[string]any
	var raw []byte
	if content {
		raw = []byte(r.Header.Get("Dropbox-API-Arg"))
		if ct := r.Header.Get("Content-Type"); r.URL.Path != "/2/files/download" && ct != "application/octet-stream" {
			dropboxPlain(w, http.StatusBadRequest, "Bad HTTP \"Content-Type\" header: \""+ct+"\". Expecting \"application/octet-stream\".")
			return
		}
	} else {
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			dropboxPlain(w, http.StatusBadRequest, "Bad HTTP \"Content-Type\" header: \""+ct+"\". Expecting \"application/json\".")
			return
		}
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			return
		}
	}
	if err := json.Unmarshal(raw, &arg); err != nil {
		dropboxPlain(w, http.StatusBadRequest, "Error in call to API function: could not decode input as JSON")
		return
	}

	var result any
	var err error
	switch r.URL.Path {
	case "/2/files/get_metadata":
		result, err = d.getMetadata(arg)
	case "/2/files/list_folder":
		result, err = d.listFolder(arg)
	case "/2/files/list_folder/continue":
		result, err = d.listContinue(arg)
	case "/2/files/create_folder", "/2/files/create_folder_v2":
		result, err = d.createFolder(arg)
	case "/2/files/delete", "/2/files/delete_v2":
		result, err = d.delete(arg)
	case "/2/files/move", "/2/files/move_v2":
		result, err = d.move(arg)
	case "/2/files/upload":
		result, err = d.upload(arg, r.Body)
	case "/2/files/upload_session/start":
		result, err = d.sessionStart(arg, r.Body)
	case "/2/files/upload_session/append", "/2/files/upload_session/append_v2":
		result, err = d.sessionAppend(r.URL.Path, arg, r.Body)
	case "/2/files/upload_session/finish":
		result, err = d.sessionFinish(arg, r.Body)
	case "/2/files/upload_session/finish_batch_v2":
		result, err = d.finishBatch(arg)
	case "/2/files/download":
		d.download(w, r, arg)
		return
	default:
		dropboxPlain(w, http.StatusBadRequest, "Unknown API function: \""+strings.TrimPrefix(r.URL.Path, "/2/")+"\"")
		return
	}
	// The v2 forms wrap the metadata the legacy ones return bare.
	if err == nil && strings.HasSuffix(r.URL.Path, "_v2") && r.URL.Path != "/2/files/upload_session/append_v2" && r.URL.Path != "/2/files/upload_session/finish_batch_v2" {
		result = map[string]any{"metadata": result}
	}
	var de *dropboxErr
	switch {
	case errors.As(err, &de):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error_summary": de.summary + "/..", "error": dropboxNest(strings.Split(de.summary, "/"), de.extra)})
	case err != nil:
		log.Printf("dropbox: %s: %v", r.URL.Path, err)
		dropboxPlain(w, http.StatusInternalServerError, err.Error())
	default:
		w.Header().Set("Content-Type", "application/json")
		if result == nil {
			fmt.Fprint(w, "null")
			return
		}
		json.NewEncoder(w).Encode(result)
	}
}

func dropboxNest(tags []string, extra map[string]any) map[string]any {
	m := map[string]any{".tag": tags[0]}
	if len(tags) > 1 {
		m[tags[0]] = dropboxNest(tags[1:], extra)
	} else {
		for k, v := range extra {
			m[k] = v
		}
	}
	return m
}

func dropboxPlain(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, msg)
}

// token answers the refresh-token grant. The app authenticates with HTTP Basic, which both
// clients' oauth2 library tries first; refusing the form-field fallback keeps a refresh that
// lost the app's credentials from passing.
func (d *dropboxFake) token(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, code, desc string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
	}
	user, pass, ok := r.BasicAuth()
	if ok {
		// golang.org/x/oauth2 query-escapes both halves before encoding them.
		user, _ = url.QueryUnescape(user)
		pass, _ = url.QueryUnescape(pass)
	}
	if !ok || user != dropboxAppKey || pass != dropboxAppSecret {
		fail(http.StatusBadRequest, "invalid_client", "Invalid client_id or client_secret")
		return
	}
	if err := r.ParseForm(); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "could not parse the body")
		return
	}
	if r.PostForm.Get("grant_type") != "refresh_token" {
		fail(http.StatusBadRequest, "unsupported_grant_type", "only refresh_token is served here")
		return
	}
	if r.PostForm.Get("refresh_token") != dropboxRefreshToken {
		fail(http.StatusBadRequest, "invalid_grant", "refresh token is malformed")
		return
	}
	b := make([]byte, 24)
	rand.Read(b)
	token := "sl." + base64.RawURLEncoding.EncodeToString(b)
	d.mu.Lock()
	d.tokens[token] = true
	d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "bearer", "expires_in": 14400})
}

func (d *dropboxFake) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	d.mu.Lock()
	defer d.mu.Unlock()
	return ok && d.tokens[token]
}

func argString(arg map[string]any, key string) string {
	s, _ := arg[key].(string)
	return s
}

// path checks a path argument: "" (the root, where allowed), an id, or absolute without
// empty, "." or ".." components. An id resolves to its file's path.
func (d *dropboxFake) path(p string, rootOK bool) (string, error) {
	if strings.HasPrefix(p, "id:") {
		d.mu.Lock()
		defer d.mu.Unlock()
		if found, ok := d.paths[p]; ok {
			return found, nil
		}
		return "", dbxErr("path/not_found")
	}
	if p == "" {
		if rootOK {
			return "", nil
		}
		return "", dbxErr("path/malformed_path")
	}
	if !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return "", dbxErr("path/malformed_path")
	}
	for _, part := range strings.Split(p[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return "", dbxErr("path/malformed_path")
		}
	}
	return p, nil
}

func (d *dropboxFake) metadata(p string, fi fs.FileInfo) map[string]any {
	id, ok := d.ids[p]
	if !ok {
		b := make([]byte, 12)
		rand.Read(b)
		id = "id:" + base64.RawURLEncoding.EncodeToString(b)
		d.ids[p], d.paths[id] = id, p
	}
	m := map[string]any{
		"name":         path.Base(p),
		"path_lower":   strings.ToLower(p),
		"path_display": p,
		"id":           id,
	}
	if fi.IsDir() {
		m[".tag"] = "folder"
		return m
	}
	t := fi.ModTime().UTC().Truncate(time.Second).Format(time.RFC3339)
	m[".tag"] = "file"
	m["client_modified"] = t
	m["server_modified"] = t
	m["rev"] = fmt.Sprintf("%015x", fi.ModTime().UnixNano())
	m["size"] = fi.Size()
	m["is_downloadable"] = true
	m["content_hash"] = d.contentHash(p, fi)
	return m
}

// contentHash is Dropbox's content_hash: SHA-256 over the SHA-256 of each 4 MiB block. rclone
// compares it with the file it uploaded or downloaded.
func (d *dropboxFake) contentHash(p string, fi fs.FileInfo) string {
	if h, ok := d.hashes[p]; ok && h.size == fi.Size() && h.mtime.Equal(fi.ModTime()) {
		return h.hash
	}
	f, err := os.Open(d.local(p))
	if err != nil {
		return ""
	}
	defer f.Close()
	outer := sha256.New()
	buf := make([]byte, dropboxHashBlock)
	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 {
			block := sha256.Sum256(buf[:n])
			outer.Write(block[:])
		}
		if err != nil {
			break
		}
	}
	h := hex.EncodeToString(outer.Sum(nil))
	d.hashes[p] = dropboxHash{size: fi.Size(), mtime: fi.ModTime(), hash: h}
	return h
}

// stat looks p up; a missing one, or one below a file, is not found.
func (d *dropboxFake) stat(p string) (fs.FileInfo, bool) {
	if p == "" {
		fi, err := os.Stat(d.root())
		return fi, err == nil
	}
	fi, err := os.Stat(d.local(p))
	return fi, err == nil
}

// parents creates p's missing parent folders, as Dropbox does for every write.
func (d *dropboxFake) parents(p, conflictTag string) error {
	at := ""
	for _, part := range strings.Split(strings.TrimPrefix(path.Dir(p), "/"), "/") {
		if part == "" {
			break
		}
		at += "/" + part
		if fi, ok := d.stat(at); ok && !fi.IsDir() {
			return dbxErr(conflictTag + "/conflict/file")
		}
	}
	return os.MkdirAll(filepath.Dir(d.local(p)), 0o755)
}

// rekey moves the ids of p and everything below it to the same names under to, or forgets
// them when to is empty.
func (d *dropboxFake) rekey(p, to string) {
	for old, id := range d.ids {
		if old != p && !strings.HasPrefix(old, p+"/") {
			continue
		}
		delete(d.ids, old)
		delete(d.paths, id)
		delete(d.hashes, old)
		if to != "" {
			d.ids[to+old[len(p):]], d.paths[id] = id, to+old[len(p):]
		}
	}
}

func (d *dropboxFake) getMetadata(arg map[string]any) (any, error) {
	p, err := d.path(argString(arg, "path"), false)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fi, ok := d.stat(p)
	if !ok {
		return nil, dbxErr("path/not_found")
	}
	return d.metadata(p, fi), nil
}

type dropboxCursor struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	Offset    int    `json:"offset"`
	Limit     int    `json:"limit"`
}

func (d *dropboxFake) listFolder(arg map[string]any) (any, error) {
	p, err := d.path(argString(arg, "path"), true)
	if err != nil {
		return nil, err
	}
	recursive, _ := arg["recursive"].(bool)
	limit := dropboxPageSize
	if l, ok := arg["limit"].(float64); ok && l >= 1 && int(l) < limit {
		limit = int(l)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fi, ok := d.stat(p)
	if !ok {
		return nil, dbxErr("path/not_found")
	}
	if !fi.IsDir() {
		return nil, dbxErr("path/not_folder")
	}
	return d.page(dropboxCursor{Path: p, Recursive: recursive, Limit: limit})
}

func (d *dropboxFake) listContinue(arg map[string]any) (any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(argString(arg, "cursor"))
	var c dropboxCursor
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Limit < 1 {
		return nil, dbxErr("reset")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if fi, ok := d.stat(c.Path); !ok || !fi.IsDir() {
		return nil, dbxErr("path/not_found")
	}
	return d.page(c)
}

// page lists from the cursor's offset in name order; a cursor is a position, so entries added
// or removed between pages can shift it, as Dropbox makes no promise about them either.
func (d *dropboxFake) page(c dropboxCursor) (any, error) {
	var names []string
	base := d.local(c.Path)
	if c.Path == "" {
		base = d.root()
	}
	err := filepath.WalkDir(base, func(file string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, file)
		if rel == "." {
			return nil
		}
		names = append(names, c.Path+"/"+filepath.ToSlash(rel))
		if e.IsDir() && !c.Recursive {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	entries := []any{}
	end := min(c.Offset+c.Limit, len(names))
	for _, n := range names[min(c.Offset, end):end] {
		if fi, ok := d.stat(n); ok {
			entries = append(entries, d.metadata(n, fi))
		}
	}
	next := c
	next.Offset = end
	cursor, _ := json.Marshal(next)
	return map[string]any{"entries": entries, "cursor": base64.RawURLEncoding.EncodeToString(cursor), "has_more": end < len(names)}, nil
}

func (d *dropboxFake) createFolder(arg map[string]any) (any, error) {
	p, err := d.path(argString(arg, "path"), false)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if fi, ok := d.stat(p); ok {
		if fi.IsDir() {
			return nil, dbxErr("path/conflict/folder")
		}
		return nil, dbxErr("path/conflict/file")
	}
	if err := d.parents(p, "path"); err != nil {
		return nil, err
	}
	if err := os.Mkdir(d.local(p), 0o755); err != nil {
		return nil, err
	}
	fi, _ := d.stat(p)
	return d.metadata(p, fi), nil
}

func (d *dropboxFake) delete(arg map[string]any) (any, error) {
	p, err := d.path(argString(arg, "path"), false)
	if err != nil {
		if de, ok := err.(*dropboxErr); ok {
			de.summary = "path_lookup/malformed_path"
		}
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fi, ok := d.stat(p)
	if !ok {
		return nil, dbxErr("path_lookup/not_found")
	}
	if rev := argString(arg, "parent_rev"); rev != "" && (fi.IsDir() || rev != fmt.Sprintf("%015x", fi.ModTime().UnixNano())) {
		return nil, dbxErr("path_lookup/not_found")
	}
	m := d.metadata(p, fi)
	if err := os.RemoveAll(d.local(p)); err != nil {
		return nil, err
	}
	d.rekey(p, "")
	return m, nil
}

func (d *dropboxFake) move(arg map[string]any) (any, error) {
	from, err := d.path(argString(arg, "from_path"), false)
	if err != nil {
		return nil, dbxErr("from_lookup/malformed_path")
	}
	to, err := d.path(argString(arg, "to_path"), false)
	if err != nil {
		return nil, dbxErr("to/malformed_path")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.stat(from); !ok {
		return nil, dbxErr("from_lookup/not_found")
	}
	if from == to {
		return nil, dbxErr("duplicated_or_nested_paths")
	}
	if strings.HasPrefix(to, from+"/") {
		return nil, dbxErr("cant_move_folder_into_itself")
	}
	if fi, ok := d.stat(to); ok {
		if fi.IsDir() {
			return nil, dbxErr("to/conflict/folder")
		}
		return nil, dbxErr("to/conflict/file")
	}
	if err := d.parents(to, "to"); err != nil {
		return nil, err
	}
	if err := os.Rename(d.local(from), d.local(to)); err != nil {
		return nil, err
	}
	d.rekey(from, to)
	fi, _ := d.stat(to)
	return d.metadata(to, fi), nil
}

// receive writes a request body to a new file in the scratch directory.
func (d *dropboxFake) receive(body io.Reader) (string, error) {
	if err := os.MkdirAll(d.scratch(), 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(d.scratch(), "part-")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if body != nil {
		if _, err := io.Copy(f, body); err != nil {
			os.Remove(f.Name())
			return "", err
		}
	}
	return f.Name(), nil
}

// commit places a received file at the commit info's path. mode is "add" (the default),
// "overwrite" or "update"; the SDK sends it as a union, go-dropbox as a bare string.
func (d *dropboxFake) commit(file string, info map[string]any, tag string) (any, error) {
	defer os.Remove(file)
	p, err := d.path(argString(info, "path"), false)
	if err != nil {
		return nil, dbxErr(tag + "/malformed_path")
	}
	mode, rev := "add", ""
	switch m := info["mode"].(type) {
	case string:
		mode = m
	case map[string]any:
		mode = argString(m, ".tag")
		rev = argString(m, "update")
	}
	modified := time.Now()
	if s := argString(info, "client_modified"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			modified = t
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if fi, ok := d.stat(p); ok {
		switch {
		case fi.IsDir():
			return nil, dbxErr(tag + "/conflict/folder")
		case mode == "add" || (mode == "update" && rev != fmt.Sprintf("%015x", fi.ModTime().UnixNano())):
			return nil, dbxErr(tag + "/conflict/file")
		}
	}
	if err := d.parents(p, tag); err != nil {
		return nil, err
	}
	if err := os.Chtimes(file, modified, modified); err != nil {
		return nil, err
	}
	if err := os.Rename(file, d.local(p)); err != nil {
		return nil, err
	}
	fi, _ := d.stat(p)
	return d.metadata(p, fi), nil
}

func (d *dropboxFake) upload(arg map[string]any, body io.Reader) (any, error) {
	file, err := d.receive(body)
	if err != nil {
		return nil, err
	}
	return d.commit(file, arg, "path")
}

func (d *dropboxFake) sessionStart(arg map[string]any, body io.Reader) (any, error) {
	file, err := d.receive(body)
	if err != nil {
		return nil, err
	}
	fi, _ := os.Stat(file)
	closed, _ := arg["close"].(bool)
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	d.mu.Lock()
	d.sessions[id] = &dropboxSession{file: file, offset: fi.Size(), closed: closed}
	d.mu.Unlock()
	return map[string]any{"session_id": id}, nil
}

// session finds an open session at the cursor's offset; tag is where the error sits in the
// route's union ("" for append, "lookup_failed" for finish).
func (d *dropboxFake) session(cursor map[string]any, tag string) (*dropboxSession, string, error) {
	id := argString(cursor, "session_id")
	prefix := ""
	if tag != "" {
		prefix = tag + "/"
	}
	s := d.sessions[id]
	if s == nil {
		return nil, id, dbxErr(prefix + "not_found")
	}
	if off, _ := cursor["offset"].(float64); int64(off) != s.offset {
		return nil, id, &dropboxErr{summary: prefix + "incorrect_offset", extra: map[string]any{"correct_offset": s.offset}}
	}
	return s, id, nil
}

func (d *dropboxFake) sessionAppend(route string, arg map[string]any, body io.Reader) (any, error) {
	cursor := arg
	if route == "/2/files/upload_session/append_v2" {
		cursor, _ = arg["cursor"].(map[string]any)
	}
	part, err := d.receive(body)
	if err != nil {
		return nil, err
	}
	defer os.Remove(part)
	d.mu.Lock()
	defer d.mu.Unlock()
	s, _, err := d.session(cursor, "")
	if err != nil {
		return nil, err
	}
	if s.closed {
		return nil, dbxErr("closed")
	}
	in, err := os.Open(part)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	out, err := os.OpenFile(s.file, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	s.offset += n
	if c, _ := arg["close"].(bool); c {
		s.closed = true
	}
	return nil, nil
}

func (d *dropboxFake) finishOne(entry map[string]any, last io.Reader) (any, error) {
	cursor, _ := entry["cursor"].(map[string]any)
	commit, _ := entry["commit"].(map[string]any)
	if last != nil {
		part, err := d.receive(last)
		if err != nil {
			return nil, err
		}
		defer os.Remove(part)
		d.mu.Lock()
		s, _, err := d.session(cursor, "lookup_failed")
		if err == nil {
			var data []byte
			if data, err = os.ReadFile(part); err == nil {
				var out *os.File
				if out, err = os.OpenFile(s.file, os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
					_, err = out.Write(data)
					out.Close()
					s.offset += int64(len(data))
				}
			}
		}
		d.mu.Unlock()
		if err != nil {
			return nil, err
		}
		cursor = map[string]any{"session_id": argString(cursor, "session_id"), "offset": float64(s.offset)}
	}
	d.mu.Lock()
	s, id, err := d.session(cursor, "lookup_failed")
	if err == nil {
		delete(d.sessions, id)
	}
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return d.commit(s.file, commit, "path")
}

func (d *dropboxFake) sessionFinish(arg map[string]any, body io.Reader) (any, error) {
	return d.finishOne(arg, body)
}

func (d *dropboxFake) finishBatch(arg map[string]any) (any, error) {
	list, _ := arg["entries"].([]any)
	results := []any{}
	for _, e := range list {
		entry, _ := e.(map[string]any)
		m, err := d.finishOne(entry, nil)
		var de *dropboxErr
		switch {
		case errors.As(err, &de):
			results = append(results, map[string]any{".tag": "failure", "failure": dropboxNest(strings.Split(de.summary, "/"), de.extra)})
		case err != nil:
			return nil, err
		default:
			success := m.(map[string]any)
			delete(success, ".tag")
			success[".tag"] = "success"
			results = append(results, success)
		}
	}
	return map[string]any{"entries": results}, nil
}

func (d *dropboxFake) download(w http.ResponseWriter, r *http.Request, arg map[string]any) {
	fail := func(summary string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error_summary": summary + "/..", "error": dropboxNest(strings.Split(summary, "/"), nil)})
	}
	p, err := d.path(argString(arg, "path"), false)
	if err != nil {
		fail("path/malformed_path")
		return
	}
	d.mu.Lock()
	fi, ok := d.stat(p)
	var f *os.File
	var meta map[string]any
	if ok && !fi.IsDir() {
		f, err = os.Open(d.local(p))
		meta = d.metadata(p, fi)
	}
	d.mu.Unlock()
	switch {
	case !ok:
		fail("path/not_found")
		return
	case fi.IsDir():
		fail("path/not_file")
		return
	case err != nil:
		fail("path/not_found")
		return
	}
	defer f.Close()
	result, _ := json.Marshal(meta)
	w.Header().Set("Dropbox-API-Result", dropboxHeaderJSON(result))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", time.Time{}, f)
}

// dropboxHeaderJSON escapes what an HTTP header cannot carry, as Dropbox does.
func dropboxHeaderJSON(b []byte) string {
	var s strings.Builder
	for _, r := range string(b) {
		if r >= 0x7f {
			if r > 0xffff {
				r1, r2 := (r-0x10000)>>10+0xd800, (r-0x10000)&0x3ff+0xdc00
				fmt.Fprintf(&s, "\\u%04x\\u%04x", r1, r2)
				continue
			}
			fmt.Fprintf(&s, "\\u%04x", r)
		} else {
			s.WriteRune(r)
		}
	}
	return s.String()
}
