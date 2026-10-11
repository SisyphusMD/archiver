package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Vectors from rclone's quickxorhash tests.
func TestOneDriveQuickXor(t *testing.T) {
	for _, v := range []struct{ in, out string }{
		{"", "AAAAAAAAAAAAAAAAAAAAAAAAAAA="},
		{"Sg==", "SgAAAAAAAAAAAAAAAQAAAAAAAAA="},
		{"h490d57Pqz5q2rtT", "h3gEHe7giWeswgdq3MYupgAAAAA="},
		{"XJZSOiNO2bmfKnTKD7fztcQX", "/6ZArHQwAidkIxefQgEdlPGAW8w="},
		{"DWAAX5/CIfrmErgZa8ot6ZraeSbu", "LR2Z0PjuRYGKQB/mhQAuMrAGZbQ="},
	} {
		in, _ := base64.StdEncoding.DecodeString(v.in)
		got, err := odQuickXor(bytes.NewReader(in))
		if err != nil || got != v.out {
			t.Errorf("quickXor(%q) = %q, %v; want %q", v.in, got, err, v.out)
		}
	}
}

func TestOneDriveContentRange(t *testing.T) {
	for h, want := range map[string]bool{"bytes 0-9/10": true, "bytes 0--1/0": true, "bytes 5-9/10": true, "bytes 0-10/10": false, "0-9/10": false, "bytes 9-5/10": false} {
		if _, _, _, ok := odContentRange(h); ok != want {
			t.Errorf("odContentRange(%q) ok = %v, want %v", h, ok, want)
		}
	}
}

// Only the configured app refreshes: another app's ID or a wrong secret fails as Microsoft's would.
func TestOneDriveTokenNeedsTheApp(t *testing.T) {
	o := &oneDrive{tokens: map[string]string{}}
	for _, c := range []struct {
		id, secret string
		want       int
	}{
		{odClientID, odClientSecret, http.StatusOK},
		{odClientID, "wrong", http.StatusBadRequest},
		{"another-app", odClientSecret, http.StatusBadRequest},
	} {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"personal-x"},
			"client_id": {c.id}, "client_secret": {c.secret}}
		r := httptest.NewRequest(http.MethodPost, "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		o.token(w, r)
		if w.Code != c.want {
			t.Errorf("client %s secret %s: %d, want %d", c.id, c.secret, w.Code, c.want)
		}
	}
}
