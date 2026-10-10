package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Microsoft's endpoints; tests point them at a fake.
var (
	graphURL      = "https://graph.microsoft.com/v1.0"
	msTokenURL    = "https://login.microsoftonline.com/common/oauth2/v2.0/token" //nolint:gosec // G101: a path or public endpoint, not a credential
	oneDriveHTTP  = &http.Client{Timeout: 30 * time.Second}
	tokenLifetime = func(expiry time.Time) bool { return expiry.IsZero() || time.Until(expiry) > time.Minute }
)

// oauthToken is the token file Duplicacy and rclone share (golang.org/x/oauth2's Token).
type oauthToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// oneDrive returns the token (JSON) and the drive's ID and type, which rclone requires and
// Duplicacy finds for itself: the named drive, else the user's own. An expired token is
// refreshed through the user's app and written back to tokenFile (Duplicacy's writable
// copy), as Duplicacy would.
func oneDrive(ctx context.Context, tokenFile, clientID, clientSecret, driveID string) (token, id, driveType string, err error) {
	b, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", "", "", err
	}
	var t oauthToken
	if err := json.Unmarshal(b, &t); err != nil {
		return "", "", "", fmt.Errorf("the OneDrive token file is not JSON: %v", err)
	}
	if !tokenLifetime(t.Expiry) || t.AccessToken == "" {
		if t, err = refreshMS(ctx, t, clientID, clientSecret); err != nil {
			return "", "", "", err
		}
		out, _ := json.Marshal(t) //nolint:gosec // G117: the token file is written on purpose, owner-only
		if err := config.WritePrivate(tokenFile, out); err != nil {
			return "", "", "", err
		}
	}
	drive := graphURL + "/me/drive"
	if driveID != "" {
		drive = graphURL + "/drives/" + url.PathEscape(driveID)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", drive+"?$select=id,driveType", nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	resp, err := oneDriveHTTP.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("finding the OneDrive drive: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("finding the OneDrive drive: %s", resp.Status)
	}
	var d struct {
		ID        string `json:"id"`
		DriveType string `json:"driveType"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil || d.ID == "" || d.DriveType == "" {
		return "", "", "", fmt.Errorf("finding the OneDrive drive: no drive ID and type in the reply")
	}
	out, _ := json.Marshal(t) //nolint:gosec // G117: the token file is written on purpose, owner-only
	return string(out), d.ID, d.DriveType, nil
}

// refreshMS trades a refresh token for a new access token at Microsoft's token endpoint. The
// credentials travel in the request body, never a URL or argv.
func refreshMS(ctx context.Context, t oauthToken, clientID, clientSecret string) (oauthToken, error) {
	if t.RefreshToken == "" {
		return t, fmt.Errorf("the OneDrive token has expired and holds no refresh token")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}, "client_id": {clientID}, "client_secret": {clientSecret}}
	req, err := http.NewRequestWithContext(ctx, "POST", msTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return t, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oneDriveHTTP.Do(req)
	if err != nil {
		return t, fmt.Errorf("refreshing the OneDrive token: %v", err)
	}
	defer resp.Body.Close()
	var r struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || resp.StatusCode != http.StatusOK || r.AccessToken == "" {
		return t, fmt.Errorf("refreshing the OneDrive token: %s", resp.Status)
	}
	t.AccessToken, t.TokenType = r.AccessToken, r.TokenType
	if r.RefreshToken != "" {
		t.RefreshToken = r.RefreshToken
	}
	t.Expiry = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	return t, nil
}
