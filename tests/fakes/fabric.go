package main

// Storage Made Easy's Enterprise File Fabric, as its two clients call it: Duplicacy (XML,
// /api/<token>/<function>/, paths where it has no id) and rclone (JSON, /api/rpc.php, ids,
// and a separate upload endpoint). Both see one tree of folders and files with numeric ids.

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
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
	fabricHost   = "fabric.test"
	fabricRootID = "0"
	fabricTime   = "2006-01-02 15:04:05"
)

type fabricNode struct {
	id, pid, name string
	folder        bool
	size          int64
	contentType   string
	modified      time.Time
	localTime     time.Time
}

type fabricUpload struct {
	pid, name, contentType string
	localTime              time.Time
	tmp                    string
	size                   int64
	received               bool
}

type fabricFS struct {
	mu       sync.Mutex
	nodes    map[string]*fabricNode
	children map[string]map[string]string // folder id -> name -> id
	next     int
	sessions map[string]bool
	uploads  map[string]*fabricUpload
}

func init() { register(newFabricFS(), fabricHost) }

func newFabricFS() *fabricFS {
	return &fabricFS{
		nodes:    map[string]*fabricNode{fabricRootID: {id: fabricRootID, folder: true, modified: time.Now()}},
		children: map[string]map[string]string{fabricRootID: {}},
		sessions: map[string]bool{},
		uploads:  map[string]*fabricUpload{},
	}
}

func (f *fabricFS) dir() string { return filepath.Join(dataDir, "fabric") }

func (f *fabricFS) content(id string) string { return filepath.Join(f.dir(), id) }

// newID returns the next id; ids only grow, which Duplicacy's listing marker relies on.
func (f *fabricFS) newID() string {
	f.next++
	return strconv.Itoa(f.next)
}

// folderID maps the empty id the clients use for the user's root to the root's own.
func folderID(id string) string {
	if id == "" {
		return fabricRootID
	}
	return id
}

// resolve walks a slash-separated path from the folder pid. Callers hold mu.
func (f *fabricFS) resolve(pid, path string) *fabricNode {
	n := f.nodes[folderID(pid)]
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		if n == nil || !n.folder {
			return nil
		}
		n = f.nodes[f.children[n.id][part]]
	}
	return n
}

// byIDOrPath finds a node by id, or by a path from the root: Duplicacy's getFile sends a path
// in fi_id.
func (f *fabricFS) byIDOrPath(v string) *fabricNode {
	if n := f.nodes[v]; n != nil {
		return n
	}
	return f.resolve(fabricRootID, v)
}

func (f *fabricFS) remove(n *fabricNode) {
	if n.folder {
		for _, id := range f.children[n.id] {
			f.remove(f.nodes[id])
		}
		delete(f.children, n.id)
	} else {
		os.Remove(f.content(n.id))
	}
	delete(f.children[n.pid], n.name)
	delete(f.nodes, n.id)
}

// place stores the file at tmp as name in folder pid, replacing a file of that name in place
// (same id, as the service keeps an overwritten file's id). Callers hold mu.
func (f *fabricFS) place(pid, name, tmp string, size int64, contentType string, localTime time.Time) (*fabricNode, string) {
	parent := f.nodes[folderID(pid)]
	if parent == nil || !parent.folder {
		return nil, "Parent folder not found."
	}
	n := f.nodes[f.children[parent.id][name]]
	if n != nil && n.folder {
		return nil, "Folder with same name already exists."
	}
	if n == nil {
		n = &fabricNode{id: f.newID(), pid: parent.id, name: name}
		f.nodes[n.id] = n
		f.children[parent.id][name] = n.id
	}
	if err := os.Rename(tmp, f.content(n.id)); err != nil {
		return nil, err.Error()
	}
	n.size, n.contentType, n.modified, n.localTime = size, contentType, time.Now(), localTime
	return n, ""
}

// spool writes r to a temporary file in the data directory, returning its path and size.
func (f *fabricFS) spool(r io.Reader) (string, int64, string, error) {
	if err := os.MkdirAll(f.dir(), 0o755); err != nil {
		return "", 0, "", err
	}
	tmp, err := os.CreateTemp(f.dir(), "upload-*")
	if err != nil {
		return "", 0, "", err
	}
	h := md5.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", 0, "", err
	}
	return tmp.Name(), size, hex.EncodeToString(h.Sum(nil)), nil
}

// fabricItem is a file or folder as both formats carry it: Duplicacy reads path (the name),
// fi_type and fi_size from XML; rclone reads the JSON with numbers as strings.
type fabricItem struct {
	ID          string `json:"fi_id" xml:"fi_id"`
	PID         string `json:"fi_pid" xml:"fi_pid"`
	Name        string `json:"fi_name" xml:"fi_name"`
	Path        string `json:"-" xml:"path"`
	Type        string `json:"fi_type" xml:"fi_type"`
	Size        string `json:"fi_size" xml:"fi_size"`
	ContentType string `json:"fi_contenttype" xml:"fi_contenttype"`
	Modified    string `json:"fi_modified" xml:"fi_modified"`
	LocalTime   string `json:"fi_localtime" xml:"fi_localtime"`
	Trash       bool   `json:"trash" xml:"-"`
	SubFolders  int64  `json:"subfolders" xml:"-"`
}

func (f *fabricFS) item(n *fabricNode) fabricItem {
	it := fabricItem{ID: n.id, PID: n.pid, Name: n.name, Path: n.name, Type: "0", Size: strconv.FormatInt(n.size, 10),
		ContentType: n.contentType, Modified: n.modified.UTC().Format(fabricTime), LocalTime: "0000-00-00 00:00:00"}
	if !n.localTime.IsZero() {
		it.LocalTime = n.localTime.UTC().Format(fabricTime)
	}
	if n.folder {
		it.Type, it.Size, it.ContentType = "1", "0", ""
		for _, id := range f.children[n.id] {
			if f.nodes[id].folder {
				it.SubFolders++
			}
		}
	}
	return it
}

// sorted lists a folder's entries in id order, which both clients' pagination follows.
func (f *fabricFS) sorted(id string) []*fabricNode {
	var out []*fabricNode
	for _, cid := range f.children[id] {
		out = append(out, f.nodes[cid])
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].id)
		b, _ := strconv.Atoi(out[j].id)
		return a < b
	})
	return out
}

type fabricStatus struct {
	Status  string `json:"status" xml:"status"`
	Message string `json:"statusmessage" xml:"statusmessage"`
}

var fabricOK = fabricStatus{"ok", "Success"}

type fabricFile struct {
	fabricStatus
	File   *fabricItem `json:"file,omitempty" xml:"file,omitempty"`
	Exists string      `json:"exists,omitempty" xml:"exists,omitempty"`
}

// fabricReply writes v as the client asked: JSON for apiformat=json, else XML. The service
// answers 200 and carries failures in status.
func fabricReply(w http.ResponseWriter, asJSON bool, v any) {
	if asJSON {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	io.WriteString(w, xml.Header)
	xml.NewEncoder(w).EncodeElement(v, xml.StartElement{Name: xml.Name{Local: "response"}})
}

func (f *fabricFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/cgi-bin/uploader/uploader1.cgi/") {
		f.receive(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		err = r.ParseMultipartForm(32 << 20)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var function, token string
	if r.URL.Path == "/api/rpc.php" {
		function, token = r.FormValue("function"), r.FormValue("token")
	} else if parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); len(parts) == 3 && parts[0] == "api" {
		token, function = parts[1], parts[2]
	} else {
		http.NotFound(w, r)
		return
	}
	asJSON := r.FormValue("apiformat") == "json"
	reply := func(v any) { fabricReply(w, asJSON, v) }

	switch function {
	case "getTokenByAuthToken":
		if r.FormValue("authtoken") == "" {
			reply(fabricStatus{"error_auth", "Invalid authentication token."})
			return
		}
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			reply(fabricStatus{"error", err.Error()})
			return
		}
		session := "fabric-session-" + hex.EncodeToString(b[:])
		f.mu.Lock()
		f.sessions[session] = true
		f.mu.Unlock()
		reply(struct {
			fabricStatus
			Token  string `json:"token" xml:"token"`
			UserID string `json:"userid" xml:"userid"`
		}{fabricOK, session, "1"})
		return
	case "getApplianceInfo":
		reply(struct {
			fabricStatus
			SiteTitle    string `json:"sitetitle" xml:"sitetitle"`
			IsAppliance  string `json:"isappliance" xml:"isappliance"`
			Version      string `json:"softwareversion" xml:"softwareversion"`
			VersionLabel string `json:"softwareversionlabel" xml:"softwareversionlabel"`
		}{fabricOK, "File Fabric (test)", "y", "2210.01", "2210.01"})
		return
	}

	f.mu.Lock()
	valid := f.sessions[token]
	f.mu.Unlock()
	if !valid {
		reply(fabricStatus{"login_token_expired", "Token is invalid or expired."})
		return
	}

	switch function {
	case "checkPathExists":
		f.mu.Lock()
		defer f.mu.Unlock()
		n := f.resolve(r.FormValue("pid"), r.FormValue("path"))
		if n == nil {
			reply(fabricFile{fabricStatus: fabricOK, Exists: "n"})
			return
		}
		it := f.item(n)
		reply(fabricFile{fabricStatus: fabricOK, File: &it, Exists: "y"})

	case "getListOfFiles":
		f.listFiles(r, reply)

	case "getFolderContents":
		f.mu.Lock()
		defer f.mu.Unlock()
		dir := f.nodes[folderID(r.FormValue("fi_pid"))]
		if dir == nil || !dir.folder {
			reply(fabricStatus{"not_found", "Folder not found."})
			return
		}
		all := f.sorted(dir.id)
		from, _ := strconv.Atoi(r.FormValue("from"))
		count, err := strconv.Atoi(r.FormValue("count"))
		if err != nil || count <= 0 {
			count = 1000
		}
		items := []fabricItem{}
		for i := from; i < len(all) && len(items) < count; i++ {
			items = append(items, f.item(all[i]))
		}
		reply(struct {
			fabricStatus
			Total string       `json:"total" xml:"total"`
			Items []fabricItem `json:"filelist" xml:"filelist>file"`
			From  string       `json:"from" xml:"from"`
			Pid   string       `json:"pid" xml:"pid"`
		}{fabricOK, strconv.Itoa(len(all)), items, strconv.Itoa(from), dir.id})

	case "doCreateNewFolder":
		f.mu.Lock()
		defer f.mu.Unlock()
		parent := f.nodes[folderID(r.FormValue("fi_pid"))]
		name := r.FormValue("fi_name")
		if parent == nil || !parent.folder || name == "" || strings.Contains(name, "/") {
			reply(fabricStatus{"error_data", "Invalid folder."})
			return
		}
		if _, exists := f.children[parent.id][name]; exists {
			reply(fabricStatus{"error_data", "Folder with same name already exists."})
			return
		}
		n := &fabricNode{id: f.newID(), pid: parent.id, name: name, folder: true, modified: time.Now()}
		f.nodes[n.id] = n
		f.children[n.id] = map[string]string{}
		f.children[parent.id][name] = n.id
		it := f.item(n)
		reply(fabricFile{fabricStatus: fabricOK, File: &it})

	case "doUploadFiles":
		f.uploadFiles(r, reply)

	case "doInitUpload":
		size, _ := strconv.ParseInt(r.FormValue("fi_size"), 10, 64)
		lt, _ := time.Parse(fabricTime, r.FormValue("fi_localtime"))
		up := &fabricUpload{pid: folderID(r.FormValue("fi_pid")), name: r.FormValue("fi_name"),
			contentType: r.FormValue("fi_contenttype"), localTime: lt, size: size}
		f.mu.Lock()
		parent := f.nodes[up.pid]
		code := "up" + f.newID()
		if parent != nil && parent.folder && up.name != "" {
			f.uploads[code] = up
		}
		f.mu.Unlock()
		if parent == nil || !parent.folder || up.name == "" {
			reply(fabricStatus{"error_data", "Invalid upload."})
			return
		}
		reply(struct {
			fabricStatus
			UploadCode          string `json:"uploadcode" xml:"uploadcode"`
			DirectUploadSupport string `json:"directuploadsupport" xml:"directuploadsupport"`
			ResumeAllowed       string `json:"resumeallowed" xml:"resumeallowed"`
		}{fabricOK, code, "n", "n"})

	case "doCompleteUpload":
		f.mu.Lock()
		defer f.mu.Unlock()
		code := r.FormValue("uploadcode")
		up := f.uploads[code]
		if up == nil || !up.received {
			reply(fabricStatus{"error_data", "Upload not found."})
			return
		}
		delete(f.uploads, code)
		n, msg := f.place(up.pid, up.name, up.tmp, up.size, up.contentType, up.localTime)
		if n == nil {
			os.Remove(up.tmp)
			reply(fabricStatus{"error_data", msg})
			return
		}
		it := f.item(n)
		reply(struct {
			fabricStatus
			UploadedSize string     `json:"uploadedsize" xml:"uploadedsize"`
			UploadedName string     `json:"uploadedname" xml:"uploadedname"`
			File         fabricItem `json:"file" xml:"file"`
		}{fabricOK, it.Size, n.name, it})

	case "doAbortUpload":
		f.mu.Lock()
		if up := f.uploads[r.FormValue("uploadcode")]; up != nil {
			if up.tmp != "" {
				os.Remove(up.tmp)
			}
			delete(f.uploads, r.FormValue("uploadcode"))
		}
		f.mu.Unlock()
		reply(fabricOK)

	case "getFile":
		f.mu.Lock()
		n := f.byIDOrPath(r.FormValue("fi_id"))
		var file *os.File
		var err error
		var modified time.Time
		if n != nil && !n.folder {
			file, err = os.Open(f.content(n.id))
			modified = n.modified
		}
		f.mu.Unlock()
		if n == nil || n.folder || err != nil {
			w.WriteHeader(http.StatusNotFound)
			fabricReply(w, asJSON, fabricStatus{"not_found", "File not found."})
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", modified, file)

	case "doDeleteFile", "doDeleteFolder":
		f.mu.Lock()
		defer f.mu.Unlock()
		n := f.nodes[r.FormValue("fi_id")]
		if n == nil || n.id == fabricRootID || n.folder != (function == "doDeleteFolder") {
			reply(fabricStatus{"not_found", "File not found."})
			return
		}
		f.remove(n)
		reply(struct {
			fabricStatus
			ID string `json:"fi_id" xml:"fi_id"`
		}{fabricOK, n.id})

	case "doRenameFile", "doRenameFolder":
		f.mu.Lock()
		defer f.mu.Unlock()
		n := f.nodes[r.FormValue("fi_id")]
		name := r.FormValue("fi_name")
		if n == nil || n.id == fabricRootID || n.folder != (function == "doRenameFolder") {
			reply(fabricStatus{"not_found", "File not found."})
			return
		}
		if name == "" || strings.Contains(name, "/") {
			reply(fabricStatus{"error_data", "Invalid name."})
			return
		}
		if msg := f.move(n, n.pid, name); msg != "" {
			reply(fabricStatus{"error_data", msg})
			return
		}
		it := f.item(n)
		reply(fabricFile{fabricStatus: fabricOK, File: &it})

	case "doMoveFiles", "doMoveFolders":
		f.mu.Lock()
		defer f.mu.Unlock()
		dest := f.nodes[folderID(r.FormValue("dir_id"))]
		if dest == nil || !dest.folder {
			reply(fabricStatus{"not_found", "Folder not found."})
			return
		}
		var last *fabricNode
		for _, id := range strings.Split(r.FormValue("fi_ids"), ",") {
			n := f.nodes[strings.TrimSpace(id)]
			if n == nil || n.id == fabricRootID || n.folder != (function == "doMoveFolders") {
				reply(fabricStatus{"not_found", "File not found."})
				return
			}
			if msg := f.move(n, dest.id, n.name); msg != "" {
				reply(fabricStatus{"error_data", msg})
				return
			}
			last = n
		}
		it := f.item(last)
		reply(struct {
			fabricStatus
			Moved string     `json:"moved" xml:"moved"`
			File  fabricItem `json:"file" xml:"file"`
		}{fabricOK, "y", it})

	case "doModifyFile":
		f.mu.Lock()
		defer f.mu.Unlock()
		n := f.nodes[r.FormValue("fi_id")]
		if n == nil || n.folder {
			reply(fabricStatus{"not_found", "File not found."})
			return
		}
		for _, line := range strings.Split(r.FormValue("data"), "\n") {
			k, v, _ := strings.Cut(line, "=")
			switch k {
			case "fi_contenttype":
				n.contentType = v
			case "fi_localtime":
				if t, err := time.Parse(fabricTime, v); err == nil {
					n.localTime = t
				}
			}
		}
		it := f.item(n)
		reply(fabricFile{fabricStatus: fabricOK, File: &it})

	default:
		log.Printf("fabric: unknown function %s", function)
		reply(fabricStatus{"error", "Unknown function " + function + "."})
	}
}

// move renames n and/or moves it into folder pid. A file it lands on is replaced, as a
// rename over the live kit and a resurrected fossil both expect. Callers hold mu.
func (f *fabricFS) move(n *fabricNode, pid, name string) string {
	for p := f.nodes[pid]; p != nil && n.folder; p = f.nodes[p.pid] {
		if p.id == n.id {
			return "Cannot move a folder into itself."
		}
		if p.id == fabricRootID {
			break
		}
	}
	if existing := f.nodes[f.children[pid][name]]; existing != nil && existing != n {
		if existing.folder || n.folder {
			return "An item with same name already exists."
		}
		f.remove(existing)
	}
	delete(f.children[n.pid], n.name)
	n.pid, n.name, n.modified = pid, name, time.Now()
	f.children[pid][name] = n.id
	return ""
}

// listFiles is Duplicacy's getListOfFiles: files (and folders on request) after the id given
// as marker, limit at a time, truncated=1 while more remain.
func (f *fabricFS) listFiles(r *http.Request, reply func(any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dir := f.nodes[folderID(r.FormValue("fi_pid"))]
	if dir == nil || !dir.folder {
		reply(fabricStatus{"not_found", "Folder not found."})
		return
	}
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil || limit <= 0 {
		limit = 1000
	}
	marker, _ := strconv.Atoi(r.FormValue("marker"))
	folders := r.FormValue("includefolders") == "y"
	var items []fabricItem
	truncated := 0
	for _, n := range f.sorted(dir.id) {
		if id, _ := strconv.Atoi(n.id); id <= marker || (n.folder && !folders) {
			continue
		}
		if len(items) == limit {
			truncated = 1
			break
		}
		items = append(items, f.item(n))
	}
	reply(struct {
		fabricStatus
		Files     []fabricItem `json:"files" xml:"files>file"`
		Truncated int          `json:"truncated" xml:"truncated"`
	}{fabricOK, items, truncated})
}

// uploadFiles is Duplicacy's doUploadFiles: multipart file_N parts named by file_nameN, into
// the folder fi_pid.
func (f *fabricFS) uploadFiles(r *http.Request, reply func(any)) {
	if r.MultipartForm == nil {
		reply(fabricStatus{"error_data", "No files uploaded."})
		return
	}
	pid := folderID(r.FormValue("fi_pid"))
	for i := 1; ; i++ {
		fhs := r.MultipartForm.File["file_"+strconv.Itoa(i)]
		if len(fhs) == 0 {
			if i == 1 {
				reply(fabricStatus{"error_data", "No files uploaded."})
				return
			}
			break
		}
		name := r.FormValue("file_name" + strconv.Itoa(i))
		if name == "" {
			name = fhs[0].Filename
		}
		src, err := fhs[0].Open()
		if err != nil {
			reply(fabricStatus{"error", err.Error()})
			return
		}
		tmp, size, _, err := f.spool(src)
		src.Close()
		if err != nil {
			reply(fabricStatus{"error", err.Error()})
			return
		}
		f.mu.Lock()
		n, msg := f.place(pid, name, tmp, size, "application/octet-stream", time.Time{})
		f.mu.Unlock()
		if n == nil {
			os.Remove(tmp)
			reply(fabricStatus{"error_data", msg})
			return
		}
	}
	reply(fabricOK)
}

// receive is the uploader CGI rclone PUTs a file's bytes to, the upload code as the query.
func (f *fabricFS) receive(w http.ResponseWriter, r *http.Request) {
	code := r.URL.RawQuery
	name, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.URL.Path, "/cgi-bin/uploader/uploader1.cgi/"))
	f.mu.Lock()
	up := f.uploads[code]
	f.mu.Unlock()
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if up == nil || err != nil || string(name) != up.name {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"success":"n","filesize":"0","md5":""}`)
		return
	}
	tmp, size, sum, err := f.spool(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	if f.uploads[code] != up {
		f.mu.Unlock()
		os.Remove(tmp)
		http.Error(w, "upload aborted", http.StatusGone)
		return
	}
	if up.tmp != "" {
		os.Remove(up.tmp)
	}
	up.tmp, up.size, up.received = tmp, size, true
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"filesize":"%d","md5":"%s","success":"y"}`, size, sum)
}
