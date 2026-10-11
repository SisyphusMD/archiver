package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type b2Client struct {
	t     *testing.T
	s     http.Handler
	token string
}

func (c *b2Client) do(method, host, path string, hdr map[string]string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://"+host+path, bytes.NewReader(body))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	return w
}

func (c *b2Client) call(op string, in map[string]any, want int) map[string]any {
	body, _ := json.Marshal(in)
	w := c.do("POST", b2APIHost, "/b2api/v1/"+op, map[string]string{"Authorization": c.token}, body)
	if w.Code != want {
		c.t.Fatalf("%s: status %d, want %d: %s", op, w.Code, want, w.Body)
	}
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func (c *b2Client) upload(bucketID, name, data string) {
	u := c.call("b2_get_upload_url", map[string]any{"bucketId": bucketID}, 200)
	sum := sha1.Sum([]byte(data))
	w := c.do("POST", b2UploadHost, u["uploadUrl"].(string)[len("https://"+b2UploadHost):], map[string]string{
		"Authorization": u["authorizationToken"].(string), "X-Bz-File-Name": name, "Content-Type": "b2/x-auto",
		"X-Bz-Content-Sha1": hex.EncodeToString(sum[:])}, []byte(data))
	if w.Code != 200 {
		c.t.Fatalf("upload %s: %d %s", name, w.Code, w.Body)
	}
}

func names(out map[string]any) (list []string) {
	for _, f := range out["files"].([]any) {
		m := f.(map[string]any)
		list = append(list, m["fileName"].(string)+":"+m["action"].(string))
	}
	return list
}

// Duplicacy's fossil cycle: hide a chunk, find it through the version list, resurrect it by
// deleting the hide marker, and page through names.
func TestB2Versions(t *testing.T) {
	dataDir = t.TempDir()
	s := &b2Server{buckets: map[string]*b2Bucket{}, byID: map[string]*b2Version{}, tokens: map[string]bool{}, uploads: map[string]string{}}
	c := &b2Client{t: t, s: s}
	r := httptest.NewRequest("GET", "https://"+b2AuthHost+"/b2api/v4/b2_authorize_account", nil)
	r.SetBasicAuth("id", "key")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var auth struct {
		AccountID, AuthorizationToken string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &auth); err != nil || auth.AuthorizationToken == "" {
		t.Fatalf("authorize: %d %s", w.Code, w.Body)
	}
	c.token = auth.AuthorizationToken
	if w := c.do("POST", b2APIHost, "/b2api/v1/b2_list_buckets", nil, []byte(`{}`)); w.Code != 401 {
		t.Fatalf("unauthorized call answered %d", w.Code)
	}
	id := c.call("b2_create_bucket", map[string]any{"accountId": auth.AccountID, "bucketName": "bucket-one", "bucketType": "allPrivate"}, 200)["bucketId"].(string)
	c.call("b2_create_bucket", map[string]any{"accountId": auth.AccountID, "bucketName": "bucket-one", "bucketType": "allPrivate"}, 400)
	for _, n := range []string{"d/chunks/aa", "d/chunks/bb", "d/config", "d/snapshots/x/1"} {
		c.upload(id, n, "data of "+n)
	}
	c.call("b2_hide_file", map[string]any{"bucketId": id, "fileName": "d/chunks/aa"}, 200)
	c.call("b2_hide_file", map[string]any{"bucketId": id, "fileName": "d/chunks/aa"}, 400)

	if w := c.do("HEAD", b2DownloadHost, "/file/bucket-one/d/chunks/aa", map[string]string{"Authorization": c.token}, nil); w.Code != 404 {
		t.Fatalf("hidden file HEAD: %d", w.Code)
	}
	w = c.do("HEAD", b2CustomHost, "/file/bucket-one/d/config", map[string]string{"Authorization": c.token, "Range": "bytes=-1"}, nil)
	if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 15-15/16" || w.Header().Get("X-Bz-File-Id") == "" {
		t.Fatalf("ranged HEAD: %d %v", w.Code, w.Header())
	}

	got := names(c.call("b2_list_file_names", map[string]any{"bucketId": id, "prefix": "d/", "startFileName": "d/chunks"}, 200))
	if len(got) != 3 || got[0] != "d/chunks/bb:upload" {
		t.Fatalf("names: %v", got)
	}
	vers := c.call("b2_list_file_versions", map[string]any{"bucketId": id, "prefix": "d/", "startFileName": "d/chunks/aa", "maxFileCount": 1}, 200)
	if got := names(vers); len(got) != 1 || got[0] != "d/chunks/aa:hide" || vers["nextFileName"] != "d/chunks/aa" {
		t.Fatalf("versions page 1: %v next %v", got, vers["nextFileName"])
	}
	hide := vers["files"].([]any)[0].(map[string]any)["fileId"].(string)
	page2 := c.call("b2_list_file_versions", map[string]any{"bucketId": id, "prefix": "d/", "startFileName": "d/chunks/aa", "startFileId": vers["nextFileId"], "maxFileCount": 1}, 200)
	if got := names(page2); len(got) != 1 || got[0] != "d/chunks/aa:upload" {
		t.Fatalf("versions page 2: %v", got)
	}
	dirs := names(c.call("b2_list_file_names", map[string]any{"bucketId": id, "prefix": "d/", "delimiter": "/"}, 200))
	if len(dirs) != 3 || dirs[0] != "d/chunks/:folder" || dirs[2] != "d/snapshots/:folder" {
		t.Fatalf("folders: %v", dirs)
	}

	c.call("b2_delete_file_version", map[string]any{"fileName": "d/chunks/aa", "fileId": hide}, 200)
	w = c.do("GET", b2DownloadHost, "/file/bucket-one/d/chunks/aa", map[string]string{"Authorization": c.token}, nil)
	if w.Code != 200 || w.Body.String() != "data of d/chunks/aa" {
		t.Fatalf("resurrected download: %d %q", w.Code, w.Body)
	}

	// Folders paged one at a time: each resumes at the first one not yet returned.
	var paged []string
	start := any(nil)
	for page := 0; page < 10; page++ {
		in := map[string]any{"bucketId": id, "prefix": "d/", "delimiter": "/", "maxFileCount": 1}
		if start != nil {
			in["startFileName"] = start
		}
		out := c.call("b2_list_file_names", in, 200)
		paged = append(paged, names(out)...)
		if start = out["nextFileName"]; start == nil {
			break
		}
	}
	if want := "[d/chunks/:folder d/config:upload d/snapshots/:folder]"; fmt.Sprint(paged) != want {
		t.Fatalf("paged folders %v, want %s", paged, want)
	}
}
