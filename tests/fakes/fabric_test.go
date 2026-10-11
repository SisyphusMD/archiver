package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func fabricCall(t *testing.T, h http.Handler, path string, form url.Values) []byte {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "https://"+fabricHost+path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Body.Bytes()
}

// The paging neither client reaches with its 1000-entry pages in the round trip, and the two
// clients' views of one tree.
func TestFabricBothClientsShareOneTree(t *testing.T) {
	dataDir = t.TempDir()
	h := newFabricFS()

	var tok struct {
		Token string `xml:"token"`
	}
	xml.Unmarshal(fabricCall(t, h, "/api/*/getTokenByAuthToken/", url.Values{"authtoken": {"x"}}), &tok)
	if tok.Token == "" {
		t.Fatal("no session token")
	}
	api := "/api/" + tok.Token + "/"
	var folder struct {
		Status string     `xml:"status"`
		File   fabricItem `xml:"file"`
	}
	xml.Unmarshal(fabricCall(t, h, api+"doCreateNewFolder/", url.Values{"fi_pid": {"0"}, "fi_name": {"pagetest"}}), &folder)
	if folder.Status != "ok" {
		t.Fatalf("folder not created: %+v", folder)
	}

	for i := 0; i < 7; i++ {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("file_1", "f"+strconv.Itoa(i))
		part.Write([]byte("content " + strconv.Itoa(i)))
		mw.WriteField("file_name1", "f"+strconv.Itoa(i))
		mw.WriteField("fi_pid", folder.File.ID)
		mw.Close()
		r := httptest.NewRequest(http.MethodPost, "https://"+fabricHost+api+"doUploadFiles/", &body)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if !strings.Contains(w.Body.String(), "<status>ok</status>") {
			t.Fatalf("upload: %s", w.Body)
		}
	}

	var names []string
	marker := ""
	for {
		var page struct {
			Files     []fabricItem `xml:"files>file"`
			Truncated int          `xml:"truncated"`
		}
		xml.Unmarshal(fabricCall(t, h, api+"getListOfFiles/", url.Values{"fi_pid": {folder.File.ID}, "marker": {marker}, "limit": {"3"}, "includefolders": {"n"}}), &page)
		for _, f := range page.Files {
			names = append(names, f.Path)
			marker = f.ID
		}
		if page.Truncated != 1 {
			break
		}
	}
	if strings.Join(names, ",") != "f0,f1,f2,f3,f4,f5,f6" {
		t.Fatalf("listed %v", names)
	}

	rpc := func(function string, form url.Values) []byte {
		form.Set("function", function)
		form.Set("token", tok.Token)
		form.Set("apiformat", "json")
		return fabricCall(t, h, "/api/rpc.php", form)
	}
	var exists struct {
		Exists string `json:"exists"`
		File   struct {
			ID   string `json:"fi_id"`
			Size string `json:"fi_size"`
		} `json:"file"`
	}
	json.Unmarshal(rpc("checkPathExists", url.Values{"path": {"pagetest/f3"}, "pid": {""}}), &exists)
	if exists.Exists != "y" || exists.File.Size != "9" {
		t.Fatalf("checkPathExists: %+v", exists)
	}
	rpc("doRenameFile", url.Values{"fi_id": {exists.File.ID}, "fi_name": {"f4"}})

	var init struct {
		UploadCode string `json:"uploadcode"`
	}
	json.Unmarshal(rpc("doInitUpload", url.Values{"fi_pid": {folder.File.ID}, "fi_name": {"f5"}, "fi_size": {"3"}}), &init)
	r := httptest.NewRequest(http.MethodPut, "https://"+fabricHost+"/cgi-bin/uploader/uploader1.cgi/"+base64.StdEncoding.EncodeToString([]byte("f5"))+"?"+init.UploadCode, strings.NewReader("new"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"success":"y"`) {
		t.Fatalf("uploader: %s", w.Body)
	}
	if b := rpc("doCompleteUpload", url.Values{"uploadcode": {init.UploadCode}}); !bytes.Contains(b, []byte(`"status":"ok"`)) {
		t.Fatalf("complete: %s", b)
	}

	get := func(id string) string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "https://"+fabricHost+api+"getFile/", strings.NewReader(url.Values{"fi_id": {id}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(w, r)
		b, _ := io.ReadAll(w.Body)
		return string(b)
	}
	if got := get("pagetest/f4"); got != "content 3" {
		t.Fatalf("renamed file holds %q", got)
	}
	if got := get("pagetest/f5"); got != "new" {
		t.Fatalf("replaced file holds %q", got)
	}
	if got := get("pagetest/f3"); !strings.Contains(got, "not_found") {
		t.Fatalf("renamed-away path still answers %q", got)
	}
}
