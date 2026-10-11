package main

// OneDrive through Microsoft Graph, as Duplicacy's one/odb storages and rclone's onedrive
// backend call it: Microsoft's token endpoint, drives addressed by path (/root:/a/b:) and by
// item ID (/items/{id}:/rel:), paged child listings, simple and session uploads, content
// downloads by redirect, moves by PATCH and deletes. Duplicacy's personal storage still talks
// to the older api.onedrive.com, which serves the same personal drive.
//
// The test's refresh tokens name their account kind ("personal-..." or "business-..."); each
// kind is one account with its own drive, and the business account also reaches a document
// library by its drive ID. Only access tokens this fake issued are accepted, so a client that
// fails to refresh cannot pass.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	odPersonalDrive = "a1b2c3d4e5f60718"
	odBusinessDrive = "b!archivertest-own"
	odLibraryDrive  = "b!archivertest-library"
	// Pre-authenticated hosts, as the real service hands out: SharePoint for business
	// downloads and upload sessions, 1drv.com for personal downloads.
	odSharePointHost = "archivertest-my.sharepoint.com"
	odFilesHost      = "archivertest.files.1drv.com"
)

type odItem struct {
	id, name string
	parent   *odItem
	folder   bool
	children map[string]*odItem // by lower-cased name: OneDrive is case-insensitive
	size     int64
	hash     string // quickXorHash, base64
	created  time.Time
	modified time.Time
}

type odDrive struct {
	id, kind, account string
	root              *odItem
	byID              map[string]*odItem
}

type odSession struct {
	drive    *odDrive
	parent   *odItem
	name     string
	total    int64
	received int64
	file     string
	fsInfo   *odFSInfo
	replace  bool
}

type odFSInfo struct {
	Created  string `json:"createdDateTime,omitempty"`
	Modified string `json:"lastModifiedDateTime,omitempty"`
}

type oneDrive struct {
	mu        sync.Mutex
	dir       string
	next      int
	drives    map[string]*odDrive
	tokens    map[string]string // access token -> account kind
	downloads map[string]*odItem
	dlDrive   map[string]*odDrive
	sessions  map[string]*odSession
}

func init() {
	o := &oneDrive{
		drives:    map[string]*odDrive{},
		tokens:    map[string]string{},
		downloads: map[string]*odItem{},
		dlDrive:   map[string]*odDrive{},
		sessions:  map[string]*odSession{},
	}
	for _, d := range [][3]string{
		{odPersonalDrive, "personal", "personal"},
		{odBusinessDrive, "business", "business"},
		{odLibraryDrive, "documentLibrary", "business"},
	} {
		now := time.Now().UTC()
		root := &odItem{id: odRootID(d[0]), name: "root", folder: true, children: map[string]*odItem{}, created: now, modified: now}
		o.drives[d[0]] = &odDrive{id: d[0], kind: d[1], account: d[2], root: root, byID: map[string]*odItem{root.id: root}}
	}
	register(o, "login.microsoftonline.com", "graph.microsoft.com", "api.onedrive.com", odSharePointHost, odFilesHost)
}

func odRootID(drive string) string {
	if !strings.HasPrefix(drive, "b!") {
		return strings.ToUpper(drive) + "!101"
	}
	return "01ROOT" + strings.ToUpper(hex.EncodeToString([]byte(drive)))[:20]
}

func odRandom() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// odError is Graph's error body; clients branch on the status and on code (itemNotFound,
// nameAlreadyExists).
func odError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func odJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (o *oneDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	o.mu.Lock()
	if o.dir == "" {
		o.dir = filepath.Join(dataDir, "onedrive")
		_ = os.MkdirAll(o.dir, 0o755)
	}
	o.mu.Unlock()
	switch {
	case host == "login.microsoftonline.com":
		o.token(w, r)
	case (host == odFilesHost || host == odSharePointHost) && strings.HasPrefix(r.URL.Path, "/download/"):
		o.download(w, r)
	case (host == "api.onedrive.com" || host == odSharePointHost) && strings.HasPrefix(r.URL.Path, "/upload/"):
		o.uploadSession(w, r)
	case host == "graph.microsoft.com" || host == "api.onedrive.com":
		o.api(w, r, host)
	default:
		odError(w, http.StatusNotFound, "itemNotFound", "not found")
	}
}

// token is the Microsoft identity platform's v2.0 token endpoint, for the refresh grant
// clients with their own app registration make. The client may authenticate in the body or
// with basic auth (golang.org/x/oauth2 tries both).
const (
	odClientID     = "0a1b2c3d-0000-4000-8000-a2c41e7e5700"
	odClientSecret = "archiver-test-client-secret"
)

func (o *oneDrive) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		odTokenError(w, "invalid_request", err.Error())
		return
	}
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	// One app registration, the one backends.d/onedrive.sh configures: a refresh only works
	// with the app its token was issued to, so wrong credentials must fail as they would.
	if id != odClientID {
		odTokenError(w, "unauthorized_client", "AADSTS700016: Application with identifier '"+id+"' was not found.")
		return
	}
	if secret != odClientSecret {
		odTokenError(w, "invalid_client", "AADSTS7000215: Invalid client secret provided.")
		return
	}
	if g := r.PostForm.Get("grant_type"); g != "refresh_token" {
		odTokenError(w, "unsupported_grant_type", "AADSTS70003: grant type "+g+" is not supported here.")
		return
	}
	rt := r.PostForm.Get("refresh_token")
	kind, _, _ := strings.Cut(rt, "-")
	if kind != "personal" && kind != "business" {
		odTokenError(w, "invalid_grant", "AADSTS9002313: Invalid request. Request is malformed or invalid.")
		return
	}
	access := kind + "-at-" + odRandom()
	refresh := kind + "-rt-" + odRandom()
	o.mu.Lock()
	o.tokens[access] = kind
	o.mu.Unlock()
	// The test counts these lines: a client that writes its refreshed token back exchanges
	// each seeded refresh token once.
	log.Printf("onedrive: refresh token %s exchanged", rt)
	odJSON(w, http.StatusOK, map[string]any{
		"token_type": "Bearer", "scope": "Files.ReadWrite offline_access", "expires_in": 3600, "ext_expires_in": 3600,
		"access_token": access, "refresh_token": refresh,
	})
}

func odTokenError(w http.ResponseWriter, code, desc string) {
	odJSON(w, http.StatusBadRequest, map[string]string{"error": code, "error_description": desc})
}

// account is the account kind the request's bearer token was issued to.
func (o *oneDrive) account(w http.ResponseWriter, r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	tok, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || tok == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="", error="invalid_token", error_description="Access token is empty."`)
		odError(w, http.StatusUnauthorized, "InvalidAuthenticationToken", "Access token is empty.")
		return "", false
	}
	o.mu.Lock()
	kind, known := o.tokens[tok]
	o.mu.Unlock()
	if !known {
		w.Header().Set("WWW-Authenticate", `Bearer realm="", error="invalid_token", error_description="Lifetime validation failed, the token is expired."`)
		odError(w, http.StatusUnauthorized, "InvalidAuthenticationToken", "Lifetime validation failed, the token is expired.")
		return "", false
	}
	return kind, true
}

// api serves graph.microsoft.com/v1.0/{me/drive,drives/{id}}/... and the personal-only
// api.onedrive.com/v1.0/drive/...
func (o *oneDrive) api(w http.ResponseWriter, r *http.Request, host string) {
	kind, ok := o.account(w, r)
	if !ok {
		return
	}
	p, found := strings.CutPrefix(r.URL.Path, "/v1.0")
	if !found {
		odError(w, http.StatusBadRequest, "BadRequest", "Unsupported API version")
		return
	}
	var driveID, rest string
	switch {
	case host == "api.onedrive.com":
		rest, found = strings.CutPrefix(p, "/drive")
		if !found || kind != "personal" {
			odError(w, http.StatusUnauthorized, "unauthenticated", "Must be authenticated to use '/drive' syntax")
			return
		}
		driveID = odPersonalDrive
	case strings.HasPrefix(p, "/me/drive"):
		rest = strings.TrimPrefix(p, "/me/drive")
		driveID = odPersonalDrive
		if kind == "business" {
			driveID = odBusinessDrive
		}
	case strings.HasPrefix(p, "/drives/"):
		driveID, rest, _ = strings.Cut(strings.TrimPrefix(p, "/drives/"), "/")
		if rest != "" {
			rest = "/" + rest
		}
	default:
		odError(w, http.StatusBadRequest, "invalidRequest", "Invalid request")
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	d := o.drives[strings.ToLower(driveID)]
	if d == nil {
		d = o.drives[driveID]
	}
	if d == nil || d.account != kind {
		odError(w, http.StatusNotFound, "itemNotFound", "The requested drive was not found")
		return
	}
	if rest == "" || rest == "/" {
		if r.Method != http.MethodGet {
			odError(w, http.StatusMethodNotAllowed, "notSupported", "Method not allowed")
			return
		}
		odJSON(w, http.StatusOK, map[string]any{
			"id": d.id, "driveType": d.kind, "name": "OneDrive",
			"owner": map[string]any{"user": map[string]string{"displayName": "Archiver Test"}},
			"quota": map[string]any{"total": int64(1) << 40, "used": 0, "remaining": int64(1) << 40, "state": "normal"},
		})
		return
	}
	o.item(w, r, d, rest)
}

// item resolves /root..., /items/{id}... with an optional :/relative/path: and an action.
func (o *oneDrive) item(w http.ResponseWriter, r *http.Request, d *odDrive, rest string) {
	if strings.HasPrefix(rest, "/root") {
		rest = "/items/root" + strings.TrimPrefix(rest, "/root")
	}
	ref, found := strings.CutPrefix(rest, "/items/")
	if !found {
		odError(w, http.StatusBadRequest, "invalidRequest", "Invalid request")
		return
	}
	end := strings.IndexAny(ref, ":/")
	if end < 0 {
		end = len(ref)
	}
	id, ref := ref[:end], ref[end:]
	base := d.byID[id]
	if id == "root" {
		base = d.root
	}
	if base == nil {
		odError(w, http.StatusNotFound, "itemNotFound", "Item does not exist")
		return
	}
	var rel string
	if after, ok := strings.CutPrefix(ref, ":"); ok {
		after = strings.TrimPrefix(after, "/")
		if i := strings.IndexByte(after, ':'); i >= 0 {
			rel, ref = after[:i], after[i+1:]
		} else {
			rel, ref = after, ""
		}
		rel = strings.Trim(rel, "/")
	}
	action := strings.Trim(ref, "/")
	parts := []string{}
	if rel != "" {
		parts = strings.Split(rel, "/")
	}

	// Uploads and session creation name an item that may not exist yet; missing folders on
	// the way are created, as the real service does.
	if (action == "content" && r.Method == http.MethodPut) || action == "createUploadSession" {
		var parent *odItem
		var name string
		if len(parts) == 0 {
			if base.folder {
				odError(w, http.StatusBadRequest, "invalidRequest", "Cannot upload to a folder")
				return
			}
			parent, name = base.parent, base.name
		} else {
			var err error
			if parent, err = o.mkdirs(d, base, parts[:len(parts)-1]); err != nil {
				odError(w, http.StatusConflict, "nameAlreadyExists", err.Error())
				return
			}
			name = parts[len(parts)-1]
		}
		if action == "content" {
			o.simpleUpload(w, r, d, parent, name)
		} else {
			o.createSession(w, r, d, parent, name)
		}
		return
	}

	it := base
	for _, part := range parts {
		if !it.folder {
			it = nil
			break
		}
		it = it.children[strings.ToLower(part)]
		if it == nil {
			break
		}
	}
	if it == nil {
		odError(w, http.StatusNotFound, "itemNotFound", "The resource could not be found.")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		odJSON(w, http.StatusOK, o.describe(d, it))
	case action == "" && r.Method == http.MethodDelete:
		if it == d.root {
			odError(w, http.StatusForbidden, "accessDenied", "Cannot delete the root")
			return
		}
		o.remove(d, it)
		w.WriteHeader(http.StatusNoContent)
	case action == "" && r.Method == http.MethodPatch:
		o.patch(w, r, d, it)
	case action == "children" && r.Method == http.MethodGet:
		o.list(w, r, d, it)
	case action == "children" && r.Method == http.MethodPost:
		o.createFolder(w, r, d, it)
	case action == "content" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		if it.folder {
			odError(w, http.StatusBadRequest, "invalidRequest", "Cannot download a folder")
			return
		}
		tok := odRandom()
		o.downloads[tok], o.dlDrive[tok] = it, d
		dl := odFilesHost
		if d.kind != "personal" {
			dl = odSharePointHost
		}
		w.Header().Set("Location", "https://"+dl+"/download/"+tok)
		w.WriteHeader(http.StatusFound)
	default:
		odError(w, http.StatusBadRequest, "invalidRequest", "Unsupported request")
	}
}

func (o *oneDrive) newID(d *odDrive) string {
	o.next++
	if d.kind == "personal" {
		return fmt.Sprintf("%s!%d", strings.ToUpper(d.id), 1000+o.next)
	}
	return fmt.Sprintf("01ARCHIVER%016d", o.next)
}

func (o *oneDrive) blob(d *odDrive, it *odItem) string {
	return filepath.Join(o.dir, strings.ReplaceAll(d.id, "!", "_"), strings.ReplaceAll(it.id, "!", "_"))
}

func (o *oneDrive) mkdirs(d *odDrive, base *odItem, parts []string) (*odItem, error) {
	it := base
	for _, part := range parts {
		if !it.folder {
			return nil, fmt.Errorf("%s is not a folder", it.name)
		}
		c := it.children[strings.ToLower(part)]
		if c == nil {
			c = o.attach(d, it, part, true)
		}
		it = c
	}
	if !it.folder {
		return nil, fmt.Errorf("%s is not a folder", it.name)
	}
	return it, nil
}

func (o *oneDrive) attach(d *odDrive, parent *odItem, name string, folder bool) *odItem {
	now := time.Now().UTC()
	it := &odItem{id: o.newID(d), name: name, parent: parent, folder: folder, created: now, modified: now}
	if folder {
		it.children = map[string]*odItem{}
	}
	parent.children[strings.ToLower(name)] = it
	parent.modified = now
	d.byID[it.id] = it
	return it
}

func (o *oneDrive) remove(d *odDrive, it *odItem) {
	for _, c := range it.children {
		o.remove(d, c)
	}
	if !it.folder {
		_ = os.Remove(o.blob(d, it))
	}
	delete(d.byID, it.id)
	if it.parent != nil {
		delete(it.parent.children, strings.ToLower(it.name))
	}
}

func odPath(it *odItem) string {
	var names []string
	for p := it; p.parent != nil; p = p.parent {
		names = append([]string{p.name}, names...)
	}
	return strings.Join(names, "/")
}

func odTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func (o *oneDrive) describe(d *odDrive, it *odItem) map[string]any {
	m := map[string]any{
		"id": it.id, "name": it.name, "size": it.size,
		"eTag": fmt.Sprintf(`"{%s},%d"`, it.id, it.modified.UnixNano()), "cTag": fmt.Sprintf(`"c:{%s},%d"`, it.id, it.modified.UnixNano()),
		"createdDateTime": odTime(it.created), "lastModifiedDateTime": odTime(it.modified),
		"fileSystemInfo": map[string]string{"createdDateTime": odTime(it.created), "lastModifiedDateTime": odTime(it.modified)},
	}
	ref := map[string]string{"driveId": d.id, "driveType": d.kind}
	if it.parent != nil {
		ref["id"] = it.parent.id
		ref["path"] = strings.TrimSuffix("/drive/root:/"+odPath(it.parent), "/")
		if it.parent.parent == nil {
			ref["path"] = "/drive/root:"
		}
	} else {
		m["root"] = map[string]any{}
	}
	m["parentReference"] = ref
	if it.folder {
		m["folder"] = map[string]int{"childCount": len(it.children)}
	} else {
		hashes := map[string]string{"quickXorHash": it.hash}
		m["file"] = map[string]any{"mimeType": "application/octet-stream", "hashes": hashes}
	}
	return m
}

// list pages a folder's children by name with @odata.nextLink. Duplicacy asks with "top",
// rclone with "$top".
func (o *oneDrive) list(w http.ResponseWriter, r *http.Request, d *odDrive, it *odItem) {
	if !it.folder {
		odJSON(w, http.StatusOK, map[string]any{"value": []any{}})
		return
	}
	q := r.URL.Query()
	top, _ := strconv.Atoi(q.Get("$top"))
	if top <= 0 {
		top, _ = strconv.Atoi(q.Get("top"))
	}
	if top <= 0 || top > 1000 {
		top = 200
	}
	skip, _ := strconv.Atoi(q.Get("$skiptoken"))
	names := make([]string, 0, len(it.children))
	for k := range it.children {
		names = append(names, k)
	}
	sort.Strings(names)
	value := []any{}
	for i := skip; i < len(names) && i < skip+top; i++ {
		value = append(value, o.describe(d, it.children[names[i]]))
	}
	out := map[string]any{"value": value}
	if skip+top < len(names) {
		next := url.URL{Scheme: "https", Host: r.Host, Path: r.URL.Path,
			RawQuery: url.Values{"$top": {strconv.Itoa(top)}, "$skiptoken": {strconv.Itoa(skip + top)}}.Encode()}
		out["@odata.nextLink"] = next.String()
	}
	odJSON(w, http.StatusOK, out)
}

// conflict applies @microsoft.graph.conflictBehavior for a new item called name in parent:
// it reports whether to go on, with the name to use. Creating a folder fails by default.
func (o *oneDrive) conflict(w http.ResponseWriter, d *odDrive, parent *odItem, name, behavior, def string) (string, bool) {
	if behavior == "" {
		behavior = def
	}
	existing := parent.children[strings.ToLower(name)]
	if existing == nil {
		return name, true
	}
	switch behavior {
	case "replace":
		o.remove(d, existing)
		return name, true
	case "rename":
		ext := filepath.Ext(name)
		for n := 1; ; n++ {
			alt := fmt.Sprintf("%s %d%s", strings.TrimSuffix(name, ext), n, ext)
			if parent.children[strings.ToLower(alt)] == nil {
				return alt, true
			}
		}
	}
	odError(w, http.StatusConflict, "nameAlreadyExists", "An item with the same name already exists under the parent")
	return "", false
}

func (o *oneDrive) createFolder(w http.ResponseWriter, r *http.Request, d *odDrive, parent *odItem) {
	var req struct {
		Name     string          `json:"name"`
		Folder   json.RawMessage `json:"folder"`
		Behavior string          `json:"@microsoft.graph.conflictBehavior"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || strings.ContainsAny(req.Name, `/\:*?"<>|`) {
		odError(w, http.StatusBadRequest, "invalidRequest", "Invalid request body")
		return
	}
	if !parent.folder {
		odError(w, http.StatusBadRequest, "invalidRequest", "The parent is not a folder")
		return
	}
	if req.Folder == nil {
		odError(w, http.StatusBadRequest, "invalidRequest", "A folder or file facet is required")
		return
	}
	name, ok := o.conflict(w, d, parent, req.Name, req.Behavior, "fail")
	if !ok {
		return
	}
	odJSON(w, http.StatusCreated, o.describe(d, o.attach(d, parent, name, true)))
}

// store puts the file at src in parent as name (an existing file of that name is replaced
// in place, keeping its ID) and returns it with whether it is new.
func (o *oneDrive) store(d *odDrive, parent *odItem, name, src string, size int64, fsInfo *odFSInfo) (*odItem, bool, error) {
	it := parent.children[strings.ToLower(name)]
	created := it == nil
	if it != nil && it.folder {
		return nil, false, fmt.Errorf("a folder named %s exists", name)
	}
	if created {
		it = o.attach(d, parent, name, false)
	}
	dst := o.blob(d, it)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, false, err
	}
	if err := os.Rename(src, dst); err != nil {
		return nil, false, err
	}
	sum, err := odQuickXorFile(dst)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	it.size, it.hash, it.modified = size, sum, now
	if fsInfo != nil {
		if t, err := time.Parse(time.RFC3339Nano, fsInfo.Modified); err == nil {
			it.modified = t
		}
		if t, err := time.Parse(time.RFC3339Nano, fsInfo.Created); err == nil && created {
			it.created = t
		}
	}
	return it, created, nil
}

func (o *oneDrive) spool(r io.Reader) (string, int64, error) {
	f, err := os.CreateTemp(o.dir, ".spool-*")
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		_ = os.Remove(f.Name())
		return "", 0, err
	}
	return f.Name(), n, nil
}

// simpleUpload is PUT .../content: create or replace, up to 250 MB on the real service.
func (o *oneDrive) simpleUpload(w http.ResponseWriter, r *http.Request, d *odDrive, parent *odItem, name string) {
	o.mu.Unlock()
	tmp, n, err := o.spool(r.Body)
	o.mu.Lock()
	if err != nil {
		odError(w, http.StatusInternalServerError, "generalException", err.Error())
		return
	}
	if d.byID[parent.id] != parent {
		_ = os.Remove(tmp)
		odError(w, http.StatusNotFound, "itemNotFound", "The parent was deleted during the upload")
		return
	}
	it, created, err := o.store(d, parent, name, tmp, n, nil)
	if err != nil {
		_ = os.Remove(tmp)
		odError(w, http.StatusConflict, "nameAlreadyExists", err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	odJSON(w, status, o.describe(d, it))
}

func (o *oneDrive) createSession(w http.ResponseWriter, r *http.Request, d *odDrive, parent *odItem, name string) {
	var req struct {
		Item struct {
			Behavior string    `json:"@microsoft.graph.conflictBehavior"`
			FSInfo   *odFSInfo `json:"fileSystemInfo"`
		} `json:"item"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			odError(w, http.StatusBadRequest, "invalidRequest", "Invalid request body")
			return
		}
	}
	if existing := parent.children[strings.ToLower(name)]; existing != nil && req.Item.Behavior == "fail" {
		odError(w, http.StatusConflict, "nameAlreadyExists", "An item with the same name already exists under the parent")
		return
	}
	f, err := os.CreateTemp(o.dir, ".session-*")
	if err != nil {
		odError(w, http.StatusInternalServerError, "generalException", err.Error())
		return
	}
	f.Close()
	id := odRandom()
	o.sessions[id] = &odSession{drive: d, parent: parent, name: name, total: -1, file: f.Name(), fsInfo: req.Item.FSInfo}
	host := odSharePointHost
	if d.kind == "personal" {
		host = "api.onedrive.com"
	}
	odJSON(w, http.StatusOK, map[string]any{
		"uploadUrl":          "https://" + host + "/upload/" + id,
		"expirationDateTime": odTime(time.Now().Add(24 * time.Hour)),
		"nextExpectedRanges": []string{"0-"},
	})
}

// uploadSession serves an upload URL: pre-authenticated, so no bearer token is required
// (rclone sends none, Duplicacy sends its own).
func (o *oneDrive) uploadSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/upload/")
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.sessions[id]
	if s == nil {
		odError(w, http.StatusNotFound, "itemNotFound", "The upload session was not found")
		return
	}
	next := func() map[string]any {
		return map[string]any{"expirationDateTime": odTime(time.Now().Add(24 * time.Hour)), "nextExpectedRanges": []string{fmt.Sprintf("%d-", s.received)}}
	}
	switch r.Method {
	case http.MethodGet:
		odJSON(w, http.StatusOK, next())
		return
	case http.MethodDelete:
		_ = os.Remove(s.file)
		delete(o.sessions, id)
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodPut:
	default:
		odError(w, http.StatusMethodNotAllowed, "notSupported", "Method not allowed")
		return
	}
	start, end, total, ok := odContentRange(r.Header.Get("Content-Range"))
	if !ok || (s.total >= 0 && total != s.total) || end-start+1 != r.ContentLength {
		odError(w, http.StatusBadRequest, "invalidRange", "Invalid Content-Range")
		return
	}
	if start != s.received {
		odError(w, http.StatusRequestedRangeNotSatisfiable, "invalidRange", "The fragment does not start at the next expected byte")
		return
	}
	s.total = total
	o.mu.Unlock()
	f, err := os.OpenFile(s.file, os.O_WRONLY|os.O_APPEND, 0)
	var n int64
	if err == nil {
		n, err = io.Copy(f, io.LimitReader(r.Body, end-start+1))
		f.Close()
	}
	o.mu.Lock()
	if o.sessions[id] != s {
		odError(w, http.StatusNotFound, "itemNotFound", "The upload session was cancelled")
		return
	}
	if err != nil || n != end-start+1 {
		_ = os.Truncate(s.file, s.received)
		odError(w, http.StatusBadRequest, "incompleteFragment", "The fragment was not received in full")
		return
	}
	s.received += n
	if s.received < s.total {
		odJSON(w, http.StatusAccepted, next())
		return
	}
	delete(o.sessions, id)
	if s.drive.byID[s.parent.id] != s.parent {
		_ = os.Remove(s.file)
		odError(w, http.StatusNotFound, "itemNotFound", "The parent was deleted during the upload")
		return
	}
	it, created, err := o.store(s.drive, s.parent, s.name, s.file, s.received, s.fsInfo)
	if err != nil {
		_ = os.Remove(s.file)
		odError(w, http.StatusConflict, "nameAlreadyExists", err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	odJSON(w, status, o.describe(s.drive, it))
}

// odContentRange parses "bytes a-b/total"; an empty upload is "bytes 0--1/0".
func odContentRange(h string) (start, end, total int64, ok bool) {
	spec, found := strings.CutPrefix(h, "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	rng, tot, found := strings.Cut(spec, "/")
	if !found {
		return 0, 0, 0, false
	}
	a, b, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, 0, false
	}
	var err1, err2, err3 error
	start, err1 = strconv.ParseInt(a, 10, 64)
	end, err2 = strconv.ParseInt(b, 10, 64)
	total, err3 = strconv.ParseInt(tot, 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || start < 0 || end < start-1 || end >= total && total > 0 {
		return 0, 0, 0, false
	}
	return start, end, total, true
}

// patch renames and moves an item (parentReference by id or by path) and sets its
// fileSystemInfo. A move whose destination folder is missing is a 400, which Duplicacy
// answers by creating the folder.
func (o *oneDrive) patch(w http.ResponseWriter, r *http.Request, d *odDrive, it *odItem) {
	var req struct {
		Name   string `json:"name"`
		Parent *struct {
			ID      string `json:"id"`
			Path    string `json:"path"`
			DriveID string `json:"driveId"`
		} `json:"parentReference"`
		FSInfo *odFSInfo `json:"fileSystemInfo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		odError(w, http.StatusBadRequest, "invalidRequest", "Invalid request body")
		return
	}
	if it == d.root && (req.Name != "" || req.Parent != nil) {
		odError(w, http.StatusForbidden, "accessDenied", "Cannot move the root")
		return
	}
	parent, name := it.parent, it.name
	if req.Parent != nil && (req.Parent.ID != "" || req.Parent.Path != "") {
		if req.Parent.DriveID != "" && !strings.EqualFold(req.Parent.DriveID, d.id) {
			odError(w, http.StatusBadRequest, "invalidRequest", "Items cannot be moved between drives")
			return
		}
		if req.Parent.ID != "" {
			parent = d.byID[req.Parent.ID]
			if req.Parent.ID == "root" {
				parent = d.root
			}
			if parent == nil {
				odError(w, http.StatusNotFound, "itemNotFound", "The destination folder does not exist")
				return
			}
		} else {
			parent = o.byPath(d, req.Parent.Path)
			if parent == nil {
				odError(w, http.StatusBadRequest, "invalidRequest", "The destination folder does not exist")
				return
			}
		}
		if !parent.folder {
			odError(w, http.StatusBadRequest, "invalidRequest", "The destination is not a folder")
			return
		}
		for p := parent; p != nil; p = p.parent {
			if p == it {
				odError(w, http.StatusBadRequest, "invalidRequest", "Cannot move a folder into itself")
				return
			}
		}
	}
	if req.Name != "" {
		name = req.Name
	}
	if parent != it.parent || !strings.EqualFold(name, it.name) {
		if existing := parent.children[strings.ToLower(name)]; existing != nil {
			if r.URL.Query().Get("@microsoft.graph.conflictBehavior") != "replace" {
				odError(w, http.StatusConflict, "nameAlreadyExists", "An item with the same name already exists under the parent")
				return
			}
			o.remove(d, existing)
		}
	}
	delete(it.parent.children, strings.ToLower(it.name))
	it.parent.modified = time.Now().UTC()
	it.parent, it.name = parent, name
	parent.children[strings.ToLower(name)] = it
	if req.FSInfo != nil {
		if t, err := time.Parse(time.RFC3339Nano, req.FSInfo.Modified); err == nil {
			it.modified = t
		}
		if t, err := time.Parse(time.RFC3339Nano, req.FSInfo.Created); err == nil {
			it.created = t
		}
	}
	odJSON(w, http.StatusOK, o.describe(d, it))
}

// byPath resolves a parentReference path: /drive/root:/a/b or /drives/{id}/root:/a/b.
func (o *oneDrive) byPath(d *odDrive, p string) *odItem {
	_, rel, found := strings.Cut(p, "/root:")
	if !found {
		return nil
	}
	it := d.root
	for _, part := range strings.Split(strings.Trim(rel, "/"), "/") {
		if part == "" {
			continue
		}
		if it = it.children[strings.ToLower(part)]; it == nil || !it.folder {
			return nil
		}
	}
	return it
}

// download serves a pre-authenticated content URL, with Range.
func (o *oneDrive) download(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.URL.Path, "/download/")
	o.mu.Lock()
	it, d := o.downloads[tok], o.dlDrive[tok]
	var f *os.File
	var err error
	if it != nil && d.byID[it.id] == it {
		f, err = os.Open(o.blob(d, it))
	}
	o.mu.Unlock()
	if it == nil || f == nil || err != nil {
		odError(w, http.StatusNotFound, "itemNotFound", "The file was not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, it.name, it.modified, f)
}

// odQuickXor is OneDrive's QuickXorHash: bytes XORed into a 160-bit ring, each at an 11-bit
// shift from the one before, with the length XORed into the last 64 bits.
func odQuickXor(r io.Reader) (string, error) {
	const width = 160
	var data [11 * width]byte
	var size uint64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			data[size%uint64(len(data))] ^= b
			size++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	var h [21]byte
	for i := range data {
		shift := (i * 11) % width
		v := int(data[i]) << (shift % 8)
		h[shift/8] ^= byte(v)
		h[shift/8+1] ^= byte(v >> 8)
	}
	h[0] ^= h[20]
	for i := 0; i < 8; i++ {
		h[12+i] ^= byte(size >> (8 * i))
	}
	return base64.StdEncoding.EncodeToString(h[:20]), nil
}

func odQuickXorFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return odQuickXor(f)
}
