package kit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An expired token is refreshed through the user's app (secrets in the body), written back
// for Duplicacy, and used to find the drive rclone needs.
func TestOneDriveRefreshAndDrive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			r.ParseForm()
			if r.URL.RawQuery != "" || r.PostForm.Get("client_secret") != "app-secret" || r.PostForm.Get("refresh_token") != "old-refresh" {
				http.Error(w, "bad", 400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "token_type": "Bearer", "refresh_token": "new-refresh", "expires_in": 3600})
		case "/me/drive":
			if r.Header.Get("Authorization") != "Bearer new-access" {
				http.Error(w, "no", 401)
				return
			}
			w.Write([]byte(`{"id":"abc123","driveType":"personal"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	graphURL, msTokenURL = srv.URL, srv.URL+"/token"
	defer func() {
		graphURL, msTokenURL = "https://graph.microsoft.com/v1.0", "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	}()

	file := filepath.Join(t.TempDir(), "token")
	old, _ := json.Marshal(oauthToken{AccessToken: "stale", RefreshToken: "old-refresh", Expiry: time.Now().Add(-time.Hour)})
	os.WriteFile(file, old, 0o600)
	token, id, typ, err := oneDrive(file, "app-id", "app-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if id != "abc123" || typ != "personal" || !strings.Contains(token, "new-access") {
		t.Errorf("token %s id %s type %s", token, id, typ)
	}
	saved, _ := os.ReadFile(file)
	if !strings.Contains(string(saved), "new-refresh") {
		t.Errorf("refreshed token not written back: %s", saved)
	}
}
