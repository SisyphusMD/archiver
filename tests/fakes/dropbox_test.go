package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type dropboxClient struct {
	t     *testing.T
	fake  *dropboxFake
	token string
}

func newDropboxClient(t *testing.T) *dropboxClient {
	dataDir = t.TempDir()
	c := &dropboxClient{t: t, fake: newDropboxFake()}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {dropboxRefreshToken}}
	req := httptest.NewRequest("POST", "https://api.dropboxapi.com/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(dropboxAppKey, dropboxAppSecret)
	status, body, _ := c.do(req)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if status != 200 || json.Unmarshal(body, &tok) != nil || tok.AccessToken == "" {
		t.Fatalf("token: %d %s", status, body)
	}
	c.token = tok.AccessToken
	return c
}

func (c *dropboxClient) do(req *http.Request) (int, []byte, http.Header) {
	rec := httptest.NewRecorder()
	c.fake.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes(), rec.Header()
}

func (c *dropboxClient) rpc(route, arg string) (int, map[string]any) {
	req := httptest.NewRequest("POST", "https://api.dropboxapi.com/2/files/"+route, strings.NewReader(arg))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	status, body, _ := c.do(req)
	var out map[string]any
	json.Unmarshal(body, &out)
	return status, out
}

func (c *dropboxClient) content(route, arg, body string) (int, []byte, http.Header) {
	req := httptest.NewRequest("POST", "https://content.dropboxapi.com/2/files/"+route, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Dropbox-API-Arg", arg)
	if route != "download" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return c.do(req)
}

func TestDropboxTokenNeedsTheAppsCredentials(t *testing.T) {
	c := newDropboxClient(t)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {dropboxRefreshToken},
		"client_id": {dropboxAppKey}, "client_secret": {dropboxAppSecret}}
	req := httptest.NewRequest("POST", "https://api.dropboxapi.com/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if status, _, _ := c.do(req); status != 400 {
		t.Errorf("refresh without Basic credentials: %d, want 400", status)
	}
	c.token = "not-issued"
	if status, _ := c.rpc("get_metadata", `{"path":"/x"}`); status != 401 {
		t.Errorf("call with a token never issued: %d, want 401", status)
	}
}

func TestDropboxFileLifecycle(t *testing.T) {
	c := newDropboxClient(t)
	if status, _ := c.rpc("create_folder", `{"path":"/a/store"}`); status != 200 {
		t.Fatalf("create_folder: %d", status)
	}
	if _, out := c.rpc("create_folder_v2", `{"path":"/a/store"}`); !strings.HasPrefix(out["error_summary"].(string), "path/conflict/") {
		t.Errorf("second create_folder: %v", out)
	}
	status, body, _ := c.content("upload", `{"path":"/a/store/chunks/ab/cd","mode":"overwrite"}`, "hello")
	if status != 200 {
		t.Fatalf("upload: %d %s", status, body)
	}
	if _, out := c.rpc("get_metadata", `{"path":"/a/store/chunks/ab/cd"}`); out[".tag"] != "file" || out["size"] != float64(5) {
		t.Errorf("get_metadata: %v", out)
	}
	_, before := c.rpc("get_metadata", `{"path":"/a/store/chunks/ab/cd"}`)
	if _, out := c.rpc("move", `{"from_path":"/a/store/chunks/ab/cd","to_path":"/a/store/chunks/ab/cd.fsl"}`); out["name"] != "cd.fsl" || out["id"] != before["id"] {
		t.Errorf("move: %v (id before %v)", out, before["id"])
	}
	if status, out := c.rpc("get_metadata", `{"path":"/a/store/chunks/ab/cd"}`); status != 409 || !strings.HasPrefix(out["error_summary"].(string), "path/not_found/") {
		t.Errorf("moved-away file: %d %v", status, out)
	}
	status, body, hdr := c.content("download", `{"path":"`+before["id"].(string)+`"}`, "")
	if status != 200 || string(body) != "hello" {
		t.Errorf("download: %d %q", status, body)
	}
	block := sha256.Sum256([]byte("hello"))
	want := sha256.Sum256(block[:])
	var meta map[string]any
	json.Unmarshal([]byte(hdr.Get("Dropbox-API-Result")), &meta)
	if meta["content_hash"] != hex.EncodeToString(want[:]) {
		t.Errorf("content_hash %v", meta["content_hash"])
	}
	if _, out := c.rpc("delete", `{"path":"/a/store/chunks/ab/missing"}`); !strings.HasPrefix(out["error_summary"].(string), "path_lookup/not_found/") {
		t.Errorf("delete missing: %v", out)
	}
}

func TestDropboxListPagesAndSessions(t *testing.T) {
	c := newDropboxClient(t)
	for i := 0; i < dropboxPageSize+5; i++ {
		c.content("upload", `{"path":"/d/f`+strings.Repeat("x", i)+`"}`, "x")
	}
	_, out := c.rpc("list_folder", `{"path":"/d"}`)
	n := len(out["entries"].([]any))
	for out["has_more"] == true {
		_, out = c.rpc("list_folder/continue", `{"cursor":"`+out["cursor"].(string)+`"}`)
		n += len(out["entries"].([]any))
	}
	if n != dropboxPageSize+5 {
		t.Errorf("listed %d", n)
	}

	_, body, _ := c.content("upload_session/start", `{}`, "")
	var start struct {
		SessionID string `json:"session_id"`
	}
	json.Unmarshal(body, &start)
	cur := `{"session_id":"` + start.SessionID + `","offset":`
	if status, body, _ := c.content("upload_session/append_v2", `{"cursor":`+cur+`3},"close":true}`, "kit"); status != 409 || !strings.Contains(string(body), `"correct_offset":0`) {
		t.Errorf("append at a wrong offset: %d %s", status, body)
	}
	c.content("upload_session/append_v2", `{"cursor":`+cur+`0},"close":true}`, "kit")
	_, res := c.rpc("upload_session/finish_batch_v2", `{"entries":[{"cursor":`+cur+`3},"commit":{"path":"/k/kit.enc","mode":{".tag":"overwrite"}}}]}`)
	entries := res["entries"].([]any)
	if e := entries[0].(map[string]any); e[".tag"] != "success" || e["size"] != float64(3) {
		t.Errorf("finish_batch_v2: %v", res)
	}
	_, body, _ = c.content("download", `{"path":"/k/kit.enc"}`, "")
	if string(body) != "kit" {
		t.Errorf("committed %q", body)
	}
}
