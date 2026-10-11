package main

// Backblaze B2's native API as Duplicacy (b2api v1) and rclone (v4 authorization, v1 for
// the rest) call it: buckets, file versions with hide markers, uploads through handed-out
// upload URLs, and downloads by name (on the account's download host or a user's own, such
// as a CDN in front of B2) or by file ID. Duplicacy fossilizes a chunk by hiding it and
// deletes or resurrects the fossil through the version list, so versions are kept exactly.

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	b2AuthHost     = "api.backblazeb2.com"
	b2APIHost      = "api001.backblazeb2.com"
	b2DownloadHost = "f001.backblazeb2.com"
	b2UploadHost   = "pod-000-1000-00.backblazeb2.com"
	// A user's own download host (B2_DOWNLOAD_HOST of the b2-custom type).
	b2CustomHost = "b2-cdn.test"
)

type b2Version struct {
	ID          string
	Name        string
	Action      string // "upload" or "hide"
	Size        int64
	SHA1        string
	ContentType string
	Info        map[string]string
	Uploaded    int64 // milliseconds
	seq         int64
}

type b2Bucket struct {
	ID, Name, Type string
	// Each name's versions, newest first.
	files map[string][]*b2Version
}

type b2Server struct {
	mu      sync.Mutex
	seq     int64
	buckets map[string]*b2Bucket // by name
	byID    map[string]*b2Version
	tokens  map[string]bool   // account authorization tokens
	uploads map[string]string // upload token -> bucket ID
}

func init() {
	s := &b2Server{
		buckets: map[string]*b2Bucket{},
		byID:    map[string]*b2Version{},
		tokens:  map[string]bool{},
		uploads: map[string]string{},
	}
	register(s, b2AuthHost, b2APIHost, b2DownloadHost, b2UploadHost, b2CustomHost)
}

type b2Err struct {
	status        int
	code, message string
}

func b2Fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "code": code, "message": msg})
}

func b2JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

var b2APIPath = regexp.MustCompile(`^/b2api/v([1-4])/(b2_[a-z_]+)(/.*)?$`)

func (s *b2Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if strings.HasPrefix(r.URL.Path, "/file/") {
		if host != b2DownloadHost && host != b2CustomHost {
			b2Fail(w, http.StatusNotFound, "not_found", "downloads by name are served from the download URL")
			return
		}
		s.downloadByName(w, r)
		return
	}
	m := b2APIPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		b2Fail(w, http.StatusNotFound, "not_found", "no such API: "+r.URL.Path)
		return
	}
	version, op := m[1], m[2]
	// Each call belongs to the host the authorization or the upload URL handed out, so a
	// client that ignores those URLs fails here as it would against B2.
	switch {
	case op == "b2_authorize_account":
		if host != b2AuthHost {
			b2Fail(w, http.StatusNotFound, "not_found", "authorize at "+b2AuthHost)
			return
		}
		s.authorize(w, r, version)
		return
	case op == "b2_upload_file":
		if host != b2UploadHost {
			b2Fail(w, http.StatusNotFound, "not_found", "upload to the URL b2_get_upload_url returned")
			return
		}
		s.upload(w, r, strings.TrimPrefix(m[3], "/"))
		return
	case op == "b2_download_file_by_id":
		if host != b2APIHost && host != b2DownloadHost {
			b2Fail(w, http.StatusNotFound, "not_found", "download from the API or download URL")
			return
		}
		if !s.authorized(w, r) {
			return
		}
		s.mu.Lock()
		v := s.byID[r.URL.Query().Get("fileId")]
		s.mu.Unlock()
		if v == nil || v.Action != "upload" {
			b2Fail(w, http.StatusNotFound, "not_found", "file not present: "+r.URL.Query().Get("fileId"))
			return
		}
		s.serveVersion(w, r, v)
		return
	}
	if host != b2APIHost {
		b2Fail(w, http.StatusNotFound, "not_found", "call the API at the apiUrl b2_authorize_account returned")
		return
	}
	if r.Method != http.MethodPost {
		b2Fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if !s.authorized(w, r) {
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b2Fail(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	str := func(k string) string { v, _ := req[k].(string); return v }
	s.mu.Lock()
	defer s.mu.Unlock()
	var out any
	var e *b2Err
	switch op {
	case "b2_list_buckets":
		out, e = s.listBuckets(str("accountId"), str("bucketId"), str("bucketName"))
	case "b2_create_bucket":
		out, e = s.createBucket(str("bucketName"), str("bucketType"))
	case "b2_get_upload_url":
		out, e = s.uploadURL(str("bucketId"))
	case "b2_list_file_names", "b2_list_file_versions":
		count := 100
		if n, ok := req["maxFileCount"].(float64); ok && n > 0 {
			count = int(n)
		}
		out, e = s.list(str("bucketId"), str("prefix"), str("delimiter"), str("startFileName"), str("startFileId"), count, op == "b2_list_file_versions")
	case "b2_hide_file":
		out, e = s.hide(str("bucketId"), str("fileName"))
	case "b2_delete_file_version":
		out, e = s.deleteVersion(str("fileName"), str("fileId"))
	case "b2_get_file_info":
		if v := s.byID[str("fileId")]; v != nil {
			out = s.fileInfo(v)
		} else {
			e = &b2Err{http.StatusNotFound, "not_found", "file not present: " + str("fileId")}
		}
	case "b2_copy_file":
		out, e = s.copyFile(req)
	default:
		e = &b2Err{http.StatusBadRequest, "bad_request", "not supported by this stand-in: " + op}
	}
	if e != nil {
		b2Fail(w, e.status, e.code, e.message)
		return
	}
	b2JSON(w, out)
}

func (s *b2Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	ok := s.tokens[r.Header.Get("Authorization")]
	s.mu.Unlock()
	if !ok {
		b2Fail(w, http.StatusUnauthorized, "bad_auth_token", "missing or invalid authorization token")
	}
	return ok
}

func (s *b2Server) authorize(w http.ResponseWriter, r *http.Request, version string) {
	keyID, key, ok := r.BasicAuth()
	if !ok || keyID == "" || key == "" {
		b2Fail(w, http.StatusUnauthorized, "unauthorized", "a key ID and application key are required (Basic authorization)")
		return
	}
	account := fmt.Sprintf("%x", sha1.Sum([]byte(keyID)))[:12]
	s.mu.Lock()
	s.seq++
	token := fmt.Sprintf("4_%s_%08d_acct", account, s.seq)
	s.tokens[token] = true
	s.mu.Unlock()
	api := map[string]any{
		"apiUrl":                  "https://" + b2APIHost,
		"downloadUrl":             "https://" + b2DownloadHost,
		"s3ApiUrl":                "https://s3.us-west-001.backblazeb2.com",
		"recommendedPartSize":     100000000,
		"absoluteMinimumPartSize": 5000000,
	}
	out := map[string]any{"accountId": account, "authorizationToken": token}
	if version == "1" || version == "2" {
		for k, v := range api {
			out[k] = v
		}
		out["minimumPartSize"] = api["recommendedPartSize"]
		out["allowed"] = map[string]any{"bucketId": nil, "bucketName": nil, "capabilities": b2Capabilities, "namePrefix": nil}
	} else {
		api["infoType"] = "storageApi"
		if version == "3" {
			api["allowed"] = map[string]any{"bucketId": nil, "bucketName": nil, "capabilities": b2Capabilities, "namePrefix": nil}
		} else {
			api["allowed"] = map[string]any{"buckets": nil, "capabilities": b2Capabilities, "namePrefix": nil}
		}
		out["apiInfo"] = map[string]any{"storageApi": api}
		out["applicationKeyExpirationTimestamp"] = nil
	}
	b2JSON(w, out)
}

var b2Capabilities = []string{"listKeys", "writeKeys", "deleteKeys", "listBuckets", "listAllBucketNames", "readBuckets",
	"writeBuckets", "deleteBuckets", "listFiles", "readFiles", "shareFiles", "writeFiles", "deleteFiles"}

func (b *b2Bucket) json(account string) map[string]any {
	return map[string]any{"accountId": account, "bucketId": b.ID, "bucketName": b.Name, "bucketType": b.Type,
		"bucketInfo": map[string]string{}, "lifecycleRules": []any{}, "corsRules": []any{}, "revision": 1}
}

func (s *b2Server) listBuckets(account, id, name string) (any, *b2Err) {
	if account == "" {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "accountId is required"}
	}
	list := []map[string]any{}
	names := make([]string, 0, len(s.buckets))
	for n := range s.buckets {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b := s.buckets[n]
		if (id == "" || b.ID == id) && (name == "" || b.Name == name) {
			list = append(list, b.json(account))
		}
	}
	return map[string]any{"buckets": list}, nil
}

var b2BucketName = regexp.MustCompile(`^[A-Za-z0-9-]{6,63}$`)

func (s *b2Server) createBucket(name, typ string) (any, *b2Err) {
	if !b2BucketName.MatchString(name) || strings.HasPrefix(strings.ToLower(name), "b2-") {
		return nil, &b2Err{http.StatusBadRequest, "bad_bucket_name", "invalid bucket name: " + name}
	}
	if typ != "allPrivate" && typ != "allPublic" {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "bucketType must be allPrivate or allPublic"}
	}
	if _, dup := s.buckets[name]; dup {
		return nil, &b2Err{http.StatusBadRequest, "duplicate_bucket_name", "Bucket name is already in use."}
	}
	s.seq++
	b := &b2Bucket{ID: fmt.Sprintf("b2b%021d", s.seq), Name: name, Type: typ, files: map[string][]*b2Version{}}
	s.buckets[name] = b
	return b.json(""), nil
}

func (s *b2Server) bucketByID(id string) *b2Bucket {
	for _, b := range s.buckets {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func (s *b2Server) uploadURL(bucketID string) (any, *b2Err) {
	if s.bucketByID(bucketID) == nil {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "Invalid bucketId: " + bucketID}
	}
	s.seq++
	token := fmt.Sprintf("2_upload_%08d", s.seq)
	s.uploads[token] = bucketID
	return map[string]any{
		"bucketId":           bucketID,
		"uploadUrl":          fmt.Sprintf("https://%s/b2api/v1/b2_upload_file/%s/c001_v0001000_t%04d", b2UploadHost, bucketID, s.seq%10000),
		"authorizationToken": token,
	}, nil
}

// entry is a list result: one file version, or a folder when listing with a delimiter.
func (s *b2Server) entry(v *b2Version) map[string]any {
	return map[string]any{"accountId": "", "action": v.Action, "bucketId": "", "contentLength": v.Size, "size": v.Size,
		"contentSha1": v.SHA1, "contentType": v.ContentType, "fileId": v.ID, "fileInfo": v.Info, "fileName": v.Name,
		"uploadTimestamp": v.Uploaded}
}

func (s *b2Server) list(bucketID, prefix, delim, startName, startID string, count int, versions bool) (any, *b2Err) {
	b := s.bucketByID(bucketID)
	if b == nil {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "Invalid bucketId: " + bucketID}
	}
	if count > 10000 {
		count = 10000
	}
	names := make([]string, 0, len(b.files))
	for n := range b.files {
		if strings.HasPrefix(n, prefix) && n >= startName {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	files := []map[string]any{}
	var nextName, nextID any
	full := func(name, id string) bool {
		if len(files) < count {
			return false
		}
		nextName = name
		if id != "" {
			nextID = id
		}
		return true
	}
	lastFolder := ""
	for _, n := range names {
		if delim != "" {
			if i := strings.Index(n[len(prefix):], delim); i >= 0 {
				folder := n[:len(prefix)+i+len(delim)]
				if folder == lastFolder || (!versions && b.files[n][0].Action != "upload") {
					continue
				}
				if full(folder, "") {
					break
				}
				lastFolder = folder
				files = append(files, map[string]any{"action": "folder", "fileId": nil, "fileName": folder, "size": 0,
					"contentLength": 0, "uploadTimestamp": 0, "fileInfo": map[string]string{}})
				continue
			}
		}
		vs := b.files[n]
		if !versions {
			if vs[0].Action != "upload" {
				continue
			}
			vs = vs[:1]
		} else if n == startName && startID != "" {
			for i, v := range vs {
				if v.ID == startID {
					vs = vs[i:]
					break
				}
			}
		}
		stop := false
		for _, v := range vs {
			id := ""
			if versions {
				id = v.ID
			}
			if full(v.Name, id) {
				stop = true
				break
			}
			files = append(files, s.entry(v))
		}
		if stop {
			break
		}
	}
	out := map[string]any{"files": files, "nextFileName": nextName}
	if versions {
		out["nextFileId"] = nextID
	}
	return out, nil
}

func (s *b2Server) addVersion(b *b2Bucket, v *b2Version) {
	s.seq++
	v.seq = s.seq
	v.ID = fmt.Sprintf("4_z%s_f%015d", b.ID, s.seq)
	v.Uploaded = time.Now().UnixMilli()
	if v.Info == nil {
		v.Info = map[string]string{}
	}
	b.files[v.Name] = append([]*b2Version{v}, b.files[v.Name]...)
	s.byID[v.ID] = v
}

func (s *b2Server) fileInfo(v *b2Version) map[string]any {
	info := s.entry(v)
	for _, b := range s.buckets {
		if _, ok := b.files[v.Name]; ok {
			info["bucketId"] = b.ID
		}
	}
	return info
}

func (s *b2Server) hide(bucketID, name string) (any, *b2Err) {
	b := s.bucketByID(bucketID)
	if b == nil {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "Invalid bucketId: " + bucketID}
	}
	vs := b.files[name]
	if len(vs) == 0 {
		return nil, &b2Err{http.StatusBadRequest, "no_such_file", "File not present: " + name}
	}
	if vs[0].Action == "hide" {
		return nil, &b2Err{http.StatusBadRequest, "already_hidden", "File already hidden: " + name}
	}
	v := &b2Version{Name: name, Action: "hide", ContentType: "application/x-bz-hide-marker"}
	s.addVersion(b, v)
	return s.fileInfo(v), nil
}

func (s *b2Server) deleteVersion(name, id string) (any, *b2Err) {
	v := s.byID[id]
	if v == nil || v.Name != name {
		return nil, &b2Err{http.StatusBadRequest, "file_not_present", "File not present: " + name + " " + id}
	}
	for _, b := range s.buckets {
		vs := b.files[name]
		for i, x := range vs {
			if x != v {
				continue
			}
			vs = append(vs[:i:i], vs[i+1:]...)
			if len(vs) == 0 {
				delete(b.files, name)
			} else {
				b.files[name] = vs
			}
		}
	}
	delete(s.byID, id)
	if v.Action == "upload" {
		_ = os.Remove(s.blob(id))
	}
	return map[string]any{"fileId": id, "fileName": name}, nil
}

func (s *b2Server) copyFile(req map[string]any) (any, *b2Err) {
	srcID, _ := req["sourceFileId"].(string)
	name, _ := req["fileName"].(string)
	src := s.byID[srcID]
	if src == nil || src.Action != "upload" {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "source file not present: " + srcID}
	}
	if _, ok := req["range"].(string); ok {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "ranged copies are not supported by this stand-in"}
	}
	var b *b2Bucket
	if dest, _ := req["destinationBucketId"].(string); dest != "" {
		b = s.bucketByID(dest)
	} else {
		for _, x := range s.buckets {
			if _, ok := x.files[src.Name]; ok {
				b = x
			}
		}
	}
	if b == nil || name == "" {
		return nil, &b2Err{http.StatusBadRequest, "bad_request", "invalid destination"}
	}
	v := &b2Version{Name: name, Action: "upload", Size: src.Size, SHA1: src.SHA1, ContentType: src.ContentType, Info: src.Info}
	if d, _ := req["metadataDirective"].(string); d == "REPLACE" {
		v.ContentType, _ = req["contentType"].(string)
		v.Info = map[string]string{}
		if info, ok := req["fileInfo"].(map[string]any); ok {
			for k, x := range info {
				v.Info[k] = fmt.Sprint(x)
			}
		}
	}
	data, err := os.ReadFile(s.blob(srcID))
	if err != nil {
		return nil, &b2Err{http.StatusInternalServerError, "internal_error", err.Error()}
	}
	s.addVersion(b, v)
	if err := s.writeBlob(v.ID, data); err != nil {
		return nil, &b2Err{http.StatusInternalServerError, "internal_error", err.Error()}
	}
	return s.fileInfo(v), nil
}

func (s *b2Server) blob(id string) string {
	return filepath.Join(dataDir, "b2", id)
}

func (s *b2Server) writeBlob(id string, data []byte) error {
	if err := os.MkdirAll(filepath.Join(dataDir, "b2"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.blob(id), data, 0o644)
}

func (s *b2Server) upload(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodPost {
		b2Fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	s.mu.Lock()
	bucketID, ok := s.uploads[r.Header.Get("Authorization")]
	s.mu.Unlock()
	if !ok || !strings.HasPrefix(rest, bucketID+"/") {
		b2Fail(w, http.StatusUnauthorized, "bad_auth_token", "missing or invalid upload authorization token")
		return
	}
	name, err := url.QueryUnescape(r.Header.Get("X-Bz-File-Name"))
	if err != nil || name == "" || strings.HasPrefix(name, "/") {
		b2Fail(w, http.StatusBadRequest, "bad_request", "invalid X-Bz-File-Name")
		return
	}
	if r.ContentLength < 0 {
		b2Fail(w, http.StatusLengthRequired, "bad_request", "Content-Length is required")
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil || int64(len(data)) != r.ContentLength {
		b2Fail(w, http.StatusBadRequest, "bad_request", "body shorter than Content-Length")
		return
	}
	want := strings.ToLower(r.Header.Get("X-Bz-Content-Sha1"))
	switch {
	case want == "":
		b2Fail(w, http.StatusBadRequest, "bad_request", "X-Bz-Content-Sha1 is required")
		return
	case want == "hex_digits_at_end":
		if len(data) < 40 {
			b2Fail(w, http.StatusBadRequest, "bad_request", "body too short for hex_digits_at_end")
			return
		}
		want, data = string(data[len(data)-40:]), data[:len(data)-40]
	case want == "do_not_verify":
		want = ""
	}
	want = strings.TrimPrefix(want, "unverified:")
	sum := sha1.Sum(data)
	got := hex.EncodeToString(sum[:])
	if want != "" && want != got {
		b2Fail(w, http.StatusBadRequest, "bad_request", "Sha1 did not match data received")
		return
	}
	ctype := r.Header.Get("Content-Type")
	if ctype == "" {
		b2Fail(w, http.StatusBadRequest, "bad_request", "Content-Type is required")
		return
	}
	if ctype == "b2/x-auto" {
		ctype = "application/octet-stream"
	}
	info := map[string]string{}
	for k, vs := range r.Header {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-bz-info-") && len(vs) > 0 {
			val, err := url.QueryUnescape(vs[0])
			if err != nil {
				val = vs[0]
			}
			info[lk[len("x-bz-info-"):]] = val
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bucketByID(bucketID)
	if b == nil {
		b2Fail(w, http.StatusBadRequest, "bad_request", "bucket gone: "+bucketID)
		return
	}
	v := &b2Version{Name: name, Action: "upload", Size: int64(len(data)), SHA1: got, ContentType: ctype, Info: info}
	s.addVersion(b, v)
	if err := s.writeBlob(v.ID, data); err != nil {
		b2Fail(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	b2JSON(w, s.fileInfo(v))
}

func (s *b2Server) downloadByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		b2Fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	// Names arrive percent-encoded, Duplicacy's with + for a space (query escaping).
	bucket, rawName, _ := strings.Cut(strings.TrimPrefix(r.URL.EscapedPath(), "/file/"), "/")
	name, err := url.QueryUnescape(rawName)
	if err != nil {
		b2Fail(w, http.StatusBadRequest, "bad_request", "invalid file name")
		return
	}
	if bucket, err = url.PathUnescape(bucket); err != nil {
		b2Fail(w, http.StatusBadRequest, "bad_request", "invalid bucket name")
		return
	}
	s.mu.Lock()
	b := s.buckets[bucket]
	s.mu.Unlock()
	if b == nil {
		b2Fail(w, http.StatusNotFound, "not_found", "bucket not found: "+bucket)
		return
	}
	if b.Type != "allPublic" && !s.authorized(w, r) {
		return
	}
	s.mu.Lock()
	var v *b2Version
	if vs := b.files[name]; len(vs) > 0 && vs[0].Action == "upload" {
		v = vs[0]
	}
	s.mu.Unlock()
	if v == nil {
		b2Fail(w, http.StatusNotFound, "not_found", "File with such name does not exist.")
		return
	}
	s.serveVersion(w, r, v)
}

// serveVersion answers a download with B2's headers and its handling of one byte range, the
// only kind the clients ask for (Duplicacy's existence check sends bytes=-1).
func (s *b2Server) serveVersion(w http.ResponseWriter, r *http.Request, v *b2Version) {
	f, err := os.Open(s.blob(v.ID))
	if err != nil {
		b2Fail(w, http.StatusNotFound, "not_found", "file content gone: "+v.ID)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", v.ContentType)
	h.Set("Accept-Ranges", "bytes")
	h.Set("X-Bz-File-Id", v.ID)
	h.Set("X-Bz-File-Name", b2Escape(v.Name))
	h.Set("X-Bz-Content-Sha1", v.SHA1)
	h.Set("X-Bz-Upload-Timestamp", strconv.FormatInt(v.Uploaded, 10))
	for k, val := range v.Info {
		h.Set("X-Bz-Info-"+k, b2Escape(val))
	}
	start, end, status := int64(0), v.Size-1, http.StatusOK
	if rg := r.Header.Get("Range"); rg != "" && v.Size > 0 {
		var ok bool
		if start, end, ok = b2Range(rg, v.Size); !ok {
			h.Set("Content-Range", fmt.Sprintf("bytes */%d", v.Size))
			b2Fail(w, http.StatusRequestedRangeNotSatisfiable, "range_not_satisfiable", "invalid range "+rg)
			return
		}
		status = http.StatusPartialContent
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, v.Size))
	}
	h.Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := f.Seek(start, io.SeekStart); err == nil {
		_, _ = io.CopyN(w, f, end-start+1)
	}
}

// b2Range parses a single-range Range header against a file of size bytes.
func b2Range(rg string, size int64) (start, end int64, ok bool) {
	spec, found := strings.CutPrefix(rg, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	a, z, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false
	}
	if a == "" {
		n, err := strconv.ParseInt(z, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		return max(size-n, 0), size - 1, true
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start >= size {
		return 0, 0, false
	}
	end = size - 1
	if z != "" {
		if end, err = strconv.ParseInt(z, 10, 64); err != nil || end < start {
			return 0, 0, false
		}
		end = min(end, size-1)
	}
	return start, end, true
}

// b2Escape percent-encodes a name for a header, keeping the slashes.
func b2Escape(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(url.QueryEscape(p), "+", "%20")
	}
	return strings.Join(parts, "/")
}
