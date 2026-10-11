package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The queries as Duplicacy and rclone write them.
func TestDriveQuery(t *testing.T) {
	f := &driveFile{Name: "Chunks", MimeType: driveFolderMime, Parents: []string{"P1"}}
	for q, want := range map[string]bool{
		"name = 'chunks' and 'P1' in parents and trashed = false ":                                                                                                       true,
		"'P1' in parents and trashed = false and mimeType != 'application/vnd.google-apps.folder'":                                                                       false,
		"trashed=false and ('P1' in parents) and (name='Chunks') and (mimeType='application/vnd.google-apps.folder' or mimeType='application/vnd.google-apps.shortcut')": true,
		"name = 'x' and sharedWithMe and trashed = false":                                                                                                                false,
		"not 'P2' in parents": true,
		`name = 'it\'s'`:      false,
	} {
		e, err := parseDriveQuery(q)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if got, err := e.eval(fileFields{f}); err != nil || got != want {
			t.Errorf("%q = %v, %v; want %v", q, got, err, want)
		}
	}
	for _, q := range []string{"name = ", "'P1' in owners", "(name = 'a'", "size > '1'"} {
		e, err := parseDriveQuery(q)
		if err == nil {
			_, err = e.eval(fileFields{f})
		}
		if err == nil {
			t.Errorf("%q accepted", q)
		}
	}
}

func TestDriveMoveKeepsOneParent(t *testing.T) {
	dataDir = t.TempDir()
	s := services["www.googleapis.com"]
	do := func(method, path, body string) (int, map[string]any) {
		r := httptest.NewRequest(method, "https://www.googleapis.com"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer x")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	mkdir := func(name, parent string) string {
		code, out := do("POST", "/drive/v3/files", `{"name":"`+name+`","mimeType":"`+driveFolderMime+`","parents":["`+parent+`"]}`)
		if code != http.StatusOK {
			t.Fatalf("mkdir %s: %d %v", name, code, out)
		}
		return out["id"].(string)
	}
	a, b := mkdir("a", "root"), mkdir("b", "root")
	c := mkdir("c", a)
	if code, _ := do("PATCH", "/drive/v3/files/"+c+"?addParents="+b, "null"); code != http.StatusForbidden {
		t.Errorf("a second parent: %d, want 403", code)
	}
	if code, out := do("PATCH", "/drive/v3/files/"+c+"?addParents="+b+"&removeParents="+a, "null"); code != http.StatusOK {
		t.Errorf("move: %d %v", code, out)
	}
	_, out := do("GET", "/drive/v3/files?q="+url.QueryEscape("'"+b+"' in parents"), "")
	if files := out["files"].([]any); len(files) != 1 || files[0].(map[string]any)["id"] != c {
		t.Errorf("listing b: %v", out)
	}
	if code, _ := do("GET", "/drive/v3/files/"+c, ""); code != http.StatusOK {
		t.Errorf("get: %d", code)
	}
	if code, _ := do("DELETE", "/drive/v3/files/"+b, ""); code != http.StatusNoContent {
		t.Errorf("delete: %d", code)
	}
	if code, _ := do("GET", "/drive/v3/files/"+c, ""); code != http.StatusNotFound {
		t.Errorf("a deleted folder's child: %d, want 404", code)
	}
}
