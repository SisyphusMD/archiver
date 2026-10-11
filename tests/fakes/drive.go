package main

// Google Drive (archiver type gcd): the drive/v3 REST API as Duplicacy's GCD storage
// (google.golang.org/api) and rclone's drive backend call it. Files live under parents by
// ID, names need not be unique, and files.list takes Drive's query language, so lookups by
// name and listings by parent go through one small query evaluator. Metadata is held in
// memory; contents are files named by ID.

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	driveFolderMime = "application/vnd.google-apps.folder"
	// My Drive's root, which clients may also call "root".
	driveMyRoot = "0AFakeMyDriveRootFolderUk9PVA"
)

type driveFile struct {
	ID       string
	Name     string
	MimeType string
	Parents  []string
	Size     int64
	MD5      string
	SHA1     string
	SHA256   string
	Trashed  bool
	Created  time.Time
	Modified time.Time
	DriveID  string // the shared drive holding it; empty in My Drive
	Root     bool   // a drive's root folder, never listed
}

type driveShared struct {
	ID, Name, RequestID string
	Created             time.Time
}

type driveUpload struct {
	meta     map[string]json.RawMessage
	fileID   string // set when the upload replaces an existing file's content
	query    map[string][]string
	mimeType string
	data     bytes.Buffer
}

type driveService struct {
	mu      sync.Mutex
	files   map[string]*driveFile
	drives  map[string]*driveShared
	uploads map[string]*driveUpload
}

func init() {
	s := &driveService{files: map[string]*driveFile{}, drives: map[string]*driveShared{}, uploads: map[string]*driveUpload{}}
	now := time.Now().UTC()
	s.files[driveMyRoot] = &driveFile{ID: driveMyRoot, Name: "My Drive", MimeType: driveFolderMime, Created: now, Modified: now, Root: true}
	register(s, "www.googleapis.com")
}

func (s *driveService) dir() string { return filepath.Join(dataDir, "gcd") }

func driveID(prefix string, n int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return prefix + string(b)
}

// driveError writes Google's JSON error body, whose code and reason the clients branch on.
func driveError(w http.ResponseWriter, code int, reason, message string) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": code, "message": message,
		"errors": []map[string]string{{"domain": "global", "reason": reason, "message": message}},
	}})
}

func driveNotFound(w http.ResponseWriter, id string) {
	driveError(w, http.StatusNotFound, "notFound", "File not found: "+id+".")
}

func driveJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *driveFile) json() map[string]any {
	m := map[string]any{
		"kind": "drive#file", "id": f.ID, "name": f.Name, "mimeType": f.MimeType,
		"trashed": f.Trashed, "explicitlyTrashed": f.Trashed,
		"createdTime":  f.Created.Format("2006-01-02T15:04:05.000Z"),
		"modifiedTime": f.Modified.Format("2006-01-02T15:04:05.000Z"),
		"webViewLink":  "https://drive.google.com/file/d/" + f.ID + "/view",
	}
	if len(f.Parents) > 0 {
		m["parents"] = f.Parents
	}
	if f.DriveID != "" {
		m["driveId"] = f.DriveID
	}
	if f.MimeType != driveFolderMime {
		// Drive's int64 fields travel as JSON strings.
		m["size"] = strconv.FormatInt(f.Size, 10)
		m["md5Checksum"], m["sha1Checksum"], m["sha256Checksum"] = f.MD5, f.SHA1, f.SHA256
	}
	return m
}

func (s *driveService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(r.Header.Get("Authorization")) <= len("Bearer ") {
		driveError(w, http.StatusUnauthorized, "required", "Login Required.")
		return
	}
	p := r.URL.Path
	upload := strings.HasPrefix(p, "/upload/")
	p = strings.TrimPrefix(p, "/upload")
	if !strings.HasPrefix(p, "/drive/v3/") {
		driveError(w, http.StatusNotFound, "notFound", "Not Found")
		return
	}
	parts := strings.Split(strings.TrimPrefix(p, "/drive/v3/"), "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	switch {
	case upload && q.Get("upload_id") != "":
		s.uploadChunk(w, r)
	case upload && parts[0] == "files" && len(parts) <= 2 && (r.Method == http.MethodPost || r.Method == http.MethodPatch):
		id := ""
		if len(parts) == 2 {
			id = parts[1]
		}
		s.upload(w, r, id)
	case upload:
		driveError(w, http.StatusNotFound, "notFound", "Not Found")
	case parts[0] == "files" && len(parts) == 1 && r.Method == http.MethodGet:
		s.list(w, r)
	case parts[0] == "files" && len(parts) == 1 && r.Method == http.MethodPost:
		var meta map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&meta); err != nil && err != io.EOF {
			driveError(w, http.StatusBadRequest, "parseError", "Parse Error")
			return
		}
		s.create(w, q, meta, "", nil)
	case parts[0] == "files" && len(parts) == 2 && parts[1] != "":
		f := s.lookup(parts[1], q)
		if f == nil {
			driveNotFound(w, parts[1])
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.get(w, r, f)
		case http.MethodPatch:
			var meta map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&meta); err != nil && err != io.EOF {
				driveError(w, http.StatusBadRequest, "parseError", "Parse Error")
				return
			}
			s.update(w, q, f, meta, nil, "")
		case http.MethodDelete:
			if f.Root {
				driveError(w, http.StatusForbidden, "insufficientFilePermissions", "The user does not have sufficient permissions for this file.")
				return
			}
			s.remove(f.ID)
			w.WriteHeader(http.StatusNoContent)
		default:
			driveError(w, http.StatusMethodNotAllowed, "methodNotAllowed", "Method not allowed")
		}
	case parts[0] == "files":
		driveNotFound(w, "")
	case parts[0] == "about" && r.Method == http.MethodGet:
		s.about(w)
	case parts[0] == "drives" && len(parts) == 1 && r.Method == http.MethodGet:
		s.listDrives(w, r)
	case parts[0] == "drives" && len(parts) == 1 && r.Method == http.MethodPost:
		s.createDrive(w, r)
	case parts[0] == "drives" && len(parts) == 2 && r.Method == http.MethodGet:
		d := s.drives[parts[1]]
		if d == nil {
			driveError(w, http.StatusNotFound, "notFound", "Shared drive not found: "+parts[1])
			return
		}
		driveJSON(w, http.StatusOK, d.json())
	default:
		driveError(w, http.StatusNotFound, "notFound", "Not Found")
	}
}

// lookup finds a file by ID. Shared drive items are served without supportsAllDrives too:
// rclone downloads them without it, which Drive allows.
func (s *driveService) lookup(id string, _ map[string][]string) *driveFile {
	if id == "root" {
		id = driveMyRoot
	}
	return s.files[id]
}

func first(q map[string][]string, k string) string {
	if v := q[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (s *driveService) get(w http.ResponseWriter, r *http.Request, f *driveFile) {
	if r.URL.Query().Get("alt") != "media" {
		driveJSON(w, http.StatusOK, f.json())
		return
	}
	if f.MimeType == driveFolderMime {
		driveError(w, http.StatusForbidden, "fileNotDownloadable", "Only files with binary content can be downloaded.")
		return
	}
	fh, err := os.Open(filepath.Join(s.dir(), f.ID))
	if err != nil {
		driveError(w, http.StatusInternalServerError, "backendError", err.Error())
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Type", f.MimeType)
	http.ServeContent(w, r, "", f.Modified, fh)
}

// metaString reads a string field of a request's file metadata.
func metaString(meta map[string]json.RawMessage, k string) (string, bool) {
	raw, ok := meta[k]
	if !ok {
		return "", false
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}

// parentFolder resolves a parent ID for a new or moved file, or writes the error.
func (s *driveService) parentFolder(w http.ResponseWriter, id string, q map[string][]string) *driveFile {
	p := s.lookup(id, q)
	if p == nil {
		driveNotFound(w, id)
		return nil
	}
	if p.MimeType != driveFolderMime || p.Trashed {
		driveError(w, http.StatusBadRequest, "invalidParent", "The specified parent is not a folder.")
		return nil
	}
	return p
}

// create makes a file from metadata, with content when an upload carries it.
func (s *driveService) create(w http.ResponseWriter, q map[string][]string, meta map[string]json.RawMessage, mediaType string, content []byte) {
	parentIDs := []string{driveMyRoot}
	if raw, ok := meta["parents"]; ok {
		if err := json.Unmarshal(raw, &parentIDs); err != nil {
			driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
			return
		}
	}
	if len(parentIDs) != 1 {
		driveError(w, http.StatusForbidden, "cannotAddParent", "A file must have exactly one parent.")
		return
	}
	parent := s.parentFolder(w, parentIDs[0], q)
	if parent == nil {
		return
	}
	now := time.Now().UTC()
	f := &driveFile{ID: driveID("1", 32), Parents: []string{parent.ID}, Created: now, Modified: now, DriveID: parent.DriveID}
	if parent.Root && parent.ID != driveMyRoot {
		f.DriveID = parent.ID
	}
	f.Name, _ = metaString(meta, "name")
	if f.Name == "" {
		f.Name = "Untitled"
	}
	f.MimeType, _ = metaString(meta, "mimeType")
	if f.MimeType == "" {
		f.MimeType = mediaType
	}
	if f.MimeType == "" {
		f.MimeType = "application/octet-stream"
	}
	if f.MimeType == driveFolderMime && content != nil {
		driveError(w, http.StatusBadRequest, "invalid", "A folder cannot have content.")
		return
	}
	if mt, ok := metaString(meta, "modifiedTime"); ok {
		if t, err := time.Parse(time.RFC3339Nano, mt); err == nil {
			f.Modified = t.UTC()
		}
	}
	if f.MimeType != driveFolderMime {
		if err := s.store(f, content); err != nil {
			driveError(w, http.StatusInternalServerError, "backendError", err.Error())
			return
		}
	}
	s.files[f.ID] = f
	driveJSON(w, http.StatusOK, f.json())
}

func (s *driveService) store(f *driveFile, content []byte) error {
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(s.dir(), f.ID+".tmp")
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(s.dir(), f.ID)); err != nil {
		return err
	}
	m, s1, s256 := md5.Sum(content), sha1.Sum(content), sha256.Sum256(content)
	f.Size, f.MD5, f.SHA1, f.SHA256 = int64(len(content)), hex.EncodeToString(m[:]), hex.EncodeToString(s1[:]), hex.EncodeToString(s256[:])
	return nil
}

// update changes metadata, moves between parents (addParents/removeParents), and replaces
// content when an upload carries it. Drive keeps each item to exactly one parent.
func (s *driveService) update(w http.ResponseWriter, q map[string][]string, f *driveFile, meta map[string]json.RawMessage, content []byte, mediaType string) {
	if _, ok := meta["parents"]; ok {
		driveError(w, http.StatusForbidden, "fieldNotWritable", "The resource body includes fields which are not directly writable.")
		return
	}
	parents := append([]string(nil), f.Parents...)
	var newParent *driveFile
	for _, id := range splitList(first(q, "removeParents")) {
		if id == "root" {
			id = driveMyRoot
		}
		for i, p := range parents {
			if p == id {
				parents = append(parents[:i], parents[i+1:]...)
				break
			}
		}
	}
	for _, id := range splitList(first(q, "addParents")) {
		p := s.parentFolder(w, id, q)
		if p == nil {
			return
		}
		newParent = p
		if !contains(parents, p.ID) {
			parents = append(parents, p.ID)
		}
	}
	if len(parents) != 1 {
		driveError(w, http.StatusForbidden, "cannotAddParent", "Increasing the number of parents is not allowed.")
		return
	}
	if newParent != nil && s.within(newParent.ID, f.ID) {
		driveError(w, http.StatusBadRequest, "invalidParent", "A folder cannot be moved into itself or a subfolder.")
		return
	}
	if content != nil && f.MimeType == driveFolderMime {
		driveError(w, http.StatusBadRequest, "invalid", "A folder cannot have content.")
		return
	}
	if content != nil {
		if err := s.store(f, content); err != nil {
			driveError(w, http.StatusInternalServerError, "backendError", err.Error())
			return
		}
		f.Modified = time.Now().UTC()
	}
	f.Parents = parents
	if newParent != nil {
		d := newParent.DriveID
		if newParent.Root && newParent.ID != driveMyRoot {
			d = newParent.ID
		}
		s.setDrive(f, d)
	}
	if v, ok := metaString(meta, "name"); ok && v != "" {
		f.Name = v
	}
	// The metadata's type wins; without one, new content brings its own.
	if v, _ := metaString(meta, "mimeType"); v != "" {
		mediaType = v
	} else if content == nil {
		mediaType = ""
	}
	if mediaType != "" && f.MimeType != driveFolderMime && mediaType != driveFolderMime {
		f.MimeType = mediaType
	}
	if v, ok := metaString(meta, "modifiedTime"); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			f.Modified = t.UTC()
		}
	}
	if raw, ok := meta["trashed"]; ok {
		_ = json.Unmarshal(raw, &f.Trashed)
	}
	driveJSON(w, http.StatusOK, f.json())
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// within reports whether id is ancestor or lies inside it.
func (s *driveService) within(id, ancestor string) bool {
	for seen := 0; id != "" && seen < 1000; seen++ {
		if id == ancestor {
			return true
		}
		f := s.files[id]
		if f == nil || len(f.Parents) == 0 {
			return false
		}
		id = f.Parents[0]
	}
	return false
}

func (s *driveService) setDrive(f *driveFile, d string) {
	f.DriveID = d
	for _, c := range s.files {
		if contains(c.Parents, f.ID) {
			s.setDrive(c, d)
		}
	}
}

// remove deletes a file, or a folder with everything in it, skipping the trash.
func (s *driveService) remove(id string) {
	for _, c := range s.files {
		if contains(c.Parents, id) {
			s.remove(c.ID)
		}
	}
	if f := s.files[id]; f != nil && f.MimeType != driveFolderMime {
		_ = os.Remove(filepath.Join(s.dir(), id))
	}
	delete(s.files, id)
}

// upload starts an upload: multipart (metadata and content in one request), media (content
// alone), or resumable (a session the content is then sent to in ranges).
func (s *driveService) upload(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	var target *driveFile
	if id != "" {
		if target = s.lookup(id, q); target == nil {
			driveNotFound(w, id)
			return
		}
	}
	var meta map[string]json.RawMessage
	var content []byte
	var mediaType string
	switch q.Get("uploadType") {
	case "multipart":
		ct, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(ct, "multipart/") {
			driveError(w, http.StatusBadRequest, "badContent", "Unsupported content with type: "+r.Header.Get("Content-Type"))
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		part, err := mr.NextPart()
		if err != nil {
			driveError(w, http.StatusBadRequest, "badContent", "Missing metadata part")
			return
		}
		if err := json.NewDecoder(part).Decode(&meta); err != nil && err != io.EOF {
			driveError(w, http.StatusBadRequest, "parseError", "Parse Error")
			return
		}
		part, err = mr.NextPart()
		if err != nil {
			driveError(w, http.StatusBadRequest, "badContent", "Missing media part")
			return
		}
		mediaType = part.Header.Get("Content-Type")
		if content, err = io.ReadAll(part); err != nil {
			driveError(w, http.StatusBadRequest, "badContent", err.Error())
			return
		}
	case "media":
		var err error
		if content, err = io.ReadAll(r.Body); err != nil {
			driveError(w, http.StatusBadRequest, "badContent", err.Error())
			return
		}
		mediaType = r.Header.Get("Content-Type")
	case "resumable":
		if err := json.NewDecoder(r.Body).Decode(&meta); err != nil && err != io.EOF {
			driveError(w, http.StatusBadRequest, "parseError", "Parse Error")
			return
		}
		u := &driveUpload{meta: meta, fileID: id, query: q, mimeType: r.Header.Get("X-Upload-Content-Type")}
		uid := driveID("", 40)
		s.uploads[uid] = u
		loc := "https://" + r.Host + "/upload/drive/v3/files"
		if id != "" {
			loc += "/" + id
		}
		w.Header().Set("Location", loc+"?uploadType=resumable&upload_id="+uid)
		w.WriteHeader(http.StatusOK)
		return
	default:
		driveError(w, http.StatusBadRequest, "invalid", "Invalid value for: uploadType")
		return
	}
	if content == nil {
		content = []byte{}
	}
	if target != nil {
		s.update(w, q, target, meta, content, mediaType)
	} else {
		s.create(w, q, meta, mediaType, content)
	}
}

// uploadChunk takes one range of a resumable upload ("bytes a-b/total", "bytes a-b/*" or the
// status query "bytes */total"), answering 308 with the range held until the last arrives.
func (s *driveService) uploadChunk(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("upload_id")
	u := s.uploads[uid]
	if u == nil {
		driveError(w, http.StatusNotFound, "notFound", "No such upload session.")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		driveError(w, http.StatusBadRequest, "badContent", err.Error())
		return
	}
	total := int64(-1)
	cr := r.Header.Get("Content-Range")
	if cr == "" {
		total = int64(u.data.Len() + len(body))
		u.data.Write(body)
	} else {
		spec, ok := strings.CutPrefix(cr, "bytes ")
		rng, tot, ok2 := strings.Cut(spec, "/")
		if !ok || !ok2 {
			driveError(w, http.StatusBadRequest, "badRequest", "Invalid Content-Range")
			return
		}
		if tot != "*" {
			if total, err = strconv.ParseInt(tot, 10, 64); err != nil {
				driveError(w, http.StatusBadRequest, "badRequest", "Invalid Content-Range")
				return
			}
		}
		if rng != "*" {
			a, _, _ := strings.Cut(rng, "-")
			start, err := strconv.ParseInt(a, 10, 64)
			if err != nil || start > int64(u.data.Len()) {
				driveError(w, http.StatusBadRequest, "badRequest", "Invalid Content-Range")
				return
			}
			// A resent range overlaps what is held; keep the first copy of those bytes.
			if skip := int64(u.data.Len()) - start; skip < int64(len(body)) {
				u.data.Write(body[skip:])
			}
		}
	}
	if total < 0 || int64(u.data.Len()) < total {
		if u.data.Len() > 0 {
			w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", u.data.Len()-1))
		}
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}
	delete(s.uploads, uid)
	content := u.data.Bytes()[:total]
	if u.fileID != "" {
		f := s.lookup(u.fileID, u.query)
		if f == nil {
			driveNotFound(w, u.fileID)
			return
		}
		s.update(w, u.query, f, u.meta, content, u.mimeType)
		return
	}
	s.create(&statusWriter{ResponseWriter: w, ok: http.StatusCreated}, u.query, u.meta, u.mimeType, content)
}

// statusWriter turns create's 200 into the 201 a completed resumable upload answers.
type statusWriter struct {
	http.ResponseWriter
	ok int
}

func (s *statusWriter) WriteHeader(code int) {
	if code == http.StatusOK {
		code = s.ok
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *driveService) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	expr, err := parseDriveQuery(q.Get("q"))
	if err != nil {
		driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
		return
	}
	pageSize := 100
	if v := q.Get("pageSize"); v != "" {
		if pageSize, err = strconv.Atoi(v); err != nil || pageSize < 1 || pageSize > 1000 {
			driveError(w, http.StatusBadRequest, "invalid", "Invalid value for: pageSize")
			return
		}
	}
	driveFilter := q.Get("driveId")
	allDrives := q.Get("includeItemsFromAllDrives") == "true" && q.Get("supportsAllDrives") == "true"
	if driveFilter != "" && (q.Get("corpora") != "drive" || !allDrives) {
		driveError(w, http.StatusForbidden, "teamDriveIncludeItemsRequired", "The includeItemsFromAllDrives parameter must be set to true when driveId is set.")
		return
	}
	if driveFilter != "" && s.drives[driveFilter] == nil {
		driveError(w, http.StatusNotFound, "notFound", "Shared drive not found: "+driveFilter)
		return
	}
	var matched []*driveFile
	for _, f := range s.files {
		if f.Root || (driveFilter != "" && f.DriveID != driveFilter) || (driveFilter == "" && f.DriveID != "" && !allDrives) {
			continue
		}
		ok, err := expr.eval(fileFields{f})
		if err != nil {
			driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
			return
		}
		if ok {
			matched = append(matched, f)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].Created.Equal(matched[j].Created) {
			return matched[i].Created.Before(matched[j].Created)
		}
		return matched[i].ID < matched[j].ID
	})
	start := 0
	if t := q.Get("pageToken"); t != "" {
		if start, err = strconv.Atoi(strings.TrimPrefix(t, "p")); err != nil || start < 0 {
			driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
			return
		}
	}
	files := []map[string]any{}
	for i := start; i < len(matched) && i < start+pageSize; i++ {
		files = append(files, matched[i].json())
	}
	out := map[string]any{"kind": "drive#fileList", "incompleteSearch": false, "files": files}
	if start+pageSize < len(matched) {
		out["nextPageToken"] = "p" + strconv.Itoa(start+pageSize)
	}
	driveJSON(w, http.StatusOK, out)
}

func (s *driveService) about(w http.ResponseWriter) {
	var used int64
	for _, f := range s.files {
		used += f.Size
	}
	driveJSON(w, http.StatusOK, map[string]any{
		"kind": "drive#about",
		"user": map[string]any{"kind": "drive#user", "displayName": "Archiver Test", "me": true,
			"emailAddress": "archiver@archiver-test.iam.gserviceaccount.com", "permissionId": "00000000000000000001"},
		"storageQuota": map[string]string{"limit": "16106127360", "usage": strconv.FormatInt(used, 10),
			"usageInDrive": strconv.FormatInt(used, 10), "usageInDriveTrash": "0"},
		"exportFormats": map[string][]string{}, "importFormats": map[string][]string{},
	})
}

func (d *driveShared) json() map[string]any {
	return map[string]any{"kind": "drive#drive", "id": d.ID, "name": d.Name,
		"createdTime":  d.Created.Format("2006-01-02T15:04:05.000Z"),
		"capabilities": map[string]bool{"canAddChildren": true, "canDeleteChildren": true, "canListChildren": true},
		"restrictions": map[string]bool{}}
}

func (s *driveService) createDrive(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("requestId")
	if rid == "" {
		driveError(w, http.StatusBadRequest, "required", "Required parameter: requestId")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		driveError(w, http.StatusBadRequest, "required", "Required: name")
		return
	}
	for _, d := range s.drives {
		if d.RequestID == rid {
			driveError(w, http.StatusConflict, "duplicate", "A shared drive with this request ID already exists.")
			return
		}
	}
	now := time.Now().UTC()
	d := &driveShared{ID: driveID("0A", 13) + "Uk9PVA", Name: body.Name, RequestID: rid, Created: now}
	s.drives[d.ID] = d
	s.files[d.ID] = &driveFile{ID: d.ID, Name: d.Name, MimeType: driveFolderMime, Created: now, Modified: now, DriveID: d.ID, Root: true}
	driveJSON(w, http.StatusOK, d.json())
}

func (s *driveService) listDrives(w http.ResponseWriter, r *http.Request) {
	expr, err := parseDriveQuery(r.URL.Query().Get("q"))
	if err != nil {
		driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
		return
	}
	drives := []map[string]any{}
	ids := make([]string, 0, len(s.drives))
	for id := range s.drives {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ok, err := expr.eval(driveFieldsOf{s.drives[id]})
		if err != nil {
			driveError(w, http.StatusBadRequest, "invalid", "Invalid Value")
			return
		}
		if ok {
			drives = append(drives, s.drives[id].json())
		}
	}
	driveJSON(w, http.StatusOK, map[string]any{"kind": "drive#driveList", "drives": drives})
}

// Drive's query language, as far as the clients write it: terms such as
// "name = 'x'", "'id' in parents", "trashed = false", "mimeType != '...'" and a bare
// "sharedWithMe", joined by and/or/not with parentheses.

type driveFields interface {
	field(name string) (any, bool)
	inParents(id string) bool
}

type fileFields struct{ f *driveFile }

func (x fileFields) field(name string) (any, bool) {
	switch name {
	case "name":
		return x.f.Name, true
	case "mimeType":
		return x.f.MimeType, true
	case "trashed":
		return x.f.Trashed, true
	case "sharedWithMe", "starred":
		return false, true
	case "modifiedTime":
		return x.f.Modified, true
	case "createdTime":
		return x.f.Created, true
	}
	return nil, false
}

func (x fileFields) inParents(id string) bool {
	if id == "root" {
		id = driveMyRoot
	}
	return contains(x.f.Parents, id)
}

type driveFieldsOf struct{ d *driveShared }

func (x driveFieldsOf) field(name string) (any, bool) {
	switch name {
	case "name":
		return x.d.Name, true
	case "hidden":
		return false, true
	case "createdTime":
		return x.d.Created, true
	}
	return nil, false
}

func (x driveFieldsOf) inParents(string) bool { return false }

type driveExpr interface {
	eval(driveFields) (bool, error)
}

type exprAnd struct{ a, b driveExpr }
type exprOr struct{ a, b driveExpr }
type exprNot struct{ a driveExpr }
type exprTrue struct{}
type exprParents struct{ id string }
type exprCmp struct {
	field, op string
	value     any // string or bool
}

func (e exprAnd) eval(x driveFields) (bool, error) {
	a, err := e.a.eval(x)
	if err != nil || !a {
		return false, err
	}
	return e.b.eval(x)
}

func (e exprOr) eval(x driveFields) (bool, error) {
	a, err := e.a.eval(x)
	if err != nil || a {
		return a, err
	}
	return e.b.eval(x)
}

func (e exprNot) eval(x driveFields) (bool, error) {
	a, err := e.a.eval(x)
	return !a, err
}

func (exprTrue) eval(driveFields) (bool, error) { return true, nil }

func (e exprParents) eval(x driveFields) (bool, error) { return x.inParents(e.id), nil }

func (e exprCmp) eval(x driveFields) (bool, error) {
	v, ok := x.field(e.field)
	if !ok {
		return false, fmt.Errorf("unknown field %s", e.field)
	}
	switch v := v.(type) {
	case bool:
		want, ok := e.value.(bool)
		if !ok || (e.op != "=" && e.op != "!=") {
			return false, fmt.Errorf("bad comparison on %s", e.field)
		}
		return (v == want) == (e.op == "="), nil
	case string:
		want, ok := e.value.(string)
		if !ok {
			return false, fmt.Errorf("bad comparison on %s", e.field)
		}
		// Drive compares names without regard to case.
		if e.field == "name" {
			v, want = strings.ToLower(v), strings.ToLower(want)
		}
		switch e.op {
		case "=":
			return v == want, nil
		case "!=":
			return v != want, nil
		case "contains":
			return strings.Contains(v, want), nil
		}
	case time.Time:
		s, ok := e.value.(string)
		if !ok {
			return false, fmt.Errorf("bad comparison on %s", e.field)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			if t, err = time.Parse("2006-01-02T15:04:05", s); err != nil {
				return false, err
			}
		}
		c := v.Truncate(time.Second).Compare(t)
		switch e.op {
		case "=":
			return c == 0, nil
		case "!=":
			return c != 0, nil
		case "<":
			return c < 0, nil
		case "<=":
			return c <= 0, nil
		case ">":
			return c > 0, nil
		case ">=":
			return c >= 0, nil
		}
	}
	return false, fmt.Errorf("bad operator %s on %s", e.op, e.field)
}

type queryToken struct {
	kind string // "str", "word", "op", "(", ")"
	text string
}

func lexDriveQuery(q string) ([]queryToken, error) {
	var toks []queryToken
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(' || c == ')':
			toks = append(toks, queryToken{string(c), string(c)})
			i++
		case c == '\'':
			var b strings.Builder
			i++
			for ; i < len(q) && q[i] != '\''; i++ {
				if q[i] == '\\' && i+1 < len(q) {
					i++
				}
				b.WriteByte(q[i])
			}
			if i >= len(q) {
				return nil, fmt.Errorf("unterminated string")
			}
			i++
			toks = append(toks, queryToken{"str", b.String()})
		case strings.ContainsRune("=!<>", rune(c)):
			j := i + 1
			if j < len(q) && q[j] == '=' {
				j++
			}
			op := q[i:j]
			if op == "!" {
				return nil, fmt.Errorf("bad operator")
			}
			toks = append(toks, queryToken{"op", op})
			i = j
		default:
			j := i
			for j < len(q) && !strings.ContainsRune(" \t\n()='!<>", rune(q[j])) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("unexpected %q", c)
			}
			toks = append(toks, queryToken{"word", q[i:j]})
			i = j
		}
	}
	return toks, nil
}

type queryParser struct {
	toks []queryToken
	pos  int
}

func parseDriveQuery(q string) (driveExpr, error) {
	toks, err := lexDriveQuery(q)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return exprTrue{}, nil
	}
	p := &queryParser{toks: toks}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("trailing %q", p.toks[p.pos].text)
	}
	return e, nil
}

func (p *queryParser) peekWord(w string) bool {
	return p.pos < len(p.toks) && p.toks[p.pos].kind == "word" && strings.EqualFold(p.toks[p.pos].text, w)
}

func (p *queryParser) or() (driveExpr, error) {
	e, err := p.and()
	for err == nil && p.peekWord("or") {
		p.pos++
		var b driveExpr
		if b, err = p.and(); err == nil {
			e = exprOr{e, b}
		}
	}
	return e, err
}

func (p *queryParser) and() (driveExpr, error) {
	e, err := p.unary()
	for err == nil && p.peekWord("and") {
		p.pos++
		var b driveExpr
		if b, err = p.unary(); err == nil {
			e = exprAnd{e, b}
		}
	}
	return e, err
}

func (p *queryParser) unary() (driveExpr, error) {
	if p.pos >= len(p.toks) {
		return nil, fmt.Errorf("unexpected end")
	}
	if p.peekWord("not") {
		p.pos++
		e, err := p.unary()
		return exprNot{e}, err
	}
	t := p.toks[p.pos]
	p.pos++
	switch t.kind {
	case "(":
		e, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.toks) || p.toks[p.pos].kind != ")" {
			return nil, fmt.Errorf("missing )")
		}
		p.pos++
		return e, nil
	case "str":
		if !p.peekWord("in") {
			return nil, fmt.Errorf("expected in")
		}
		p.pos++
		if !p.peekWord("parents") {
			return nil, fmt.Errorf("only 'id' in parents is supported")
		}
		p.pos++
		return exprParents{t.text}, nil
	case "word":
		var op string
		switch {
		case p.pos < len(p.toks) && p.toks[p.pos].kind == "op":
			op = p.toks[p.pos].text
		case p.peekWord("contains"):
			op = "contains"
		default:
			// A bare boolean field, such as sharedWithMe.
			return exprCmp{t.text, "=", true}, nil
		}
		p.pos++
		if p.pos >= len(p.toks) {
			return nil, fmt.Errorf("missing value")
		}
		v := p.toks[p.pos]
		p.pos++
		switch {
		case v.kind == "str":
			return exprCmp{t.text, op, v.text}, nil
		case v.kind == "word" && (v.text == "true" || v.text == "false"):
			return exprCmp{t.text, op, v.text == "true"}, nil
		}
		return nil, fmt.Errorf("bad value %q", v.text)
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}
