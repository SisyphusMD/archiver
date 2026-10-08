package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Field is one STORAGE_TARGET_<N>_<Name> variable of a storage type.
type Field struct {
	Name     string // the variable's suffix, e.g. S3_BUCKETNAME
	Secret   bool   // read from a file only, never the environment
	Optional bool
	Default  string
	// Key is the credential Duplicacy reads as DUPLICACY_<STORAGE>_<KEY> (upper-cased), for a
	// secret, or for a setting Duplicacy reads the same way (Dropbox's app key).
	Key string
	// Path passes a secret's file path to Duplicacy rather than its value (a token file).
	Path bool
	// Rotates marks a token file Duplicacy rewrites as it refreshes it: Duplicacy, rclone and the
	// recovery kit use a writable copy that follows the refreshes (WritableToken), and the
	// envelope leaves it out, since paper cannot follow them.
	Rotates bool
	// BreakGlass marks a secret that may have a STORAGE_TARGET_<N>_BREAKGLASS_<Name> variant
	// for the envelope (ADR 23).
	BreakGlass bool
	Prompt     string // what init asks
}

// Type is one storage type: a Duplicacy URL scheme (ADR 23).
type Type struct {
	Name   string
	Fields []Field
	// URL is the Duplicacy storage URL for a target's values.
	URL func(v Values) string
	// CheckInterval is the default between checks (ADR 17): object storage is cheap to list;
	// file servers and consumer drives are slow or rate-limited.
	CheckInterval time.Duration
	// Require reports a missing combination the fields alone cannot express, as the name of
	// the setting to provide; empty when the values are complete.
	Require func(v Values) string
	// Remote describes the rclone remote that reaches the storage for the recovery kit
	// (ADR 24): its RCLONE_CONFIG_* settings (secrets included) and the directory on it.
	Remote func(v Values) (settings map[string]string, dir string)
}

// Values are a target's settings and secrets by field name.
type Values map[string]string

const day = 24 * time.Hour

// sub joins a container (bucket, share, drive) and an optional path inside it.
func sub(container, path string) string {
	if path = strings.Trim(path, "/"); path != "" {
		return container + "/" + path
	}
	return container
}

func s3Type(scheme string, tls bool, provider string) Type {
	return Type{
		Name: scheme,
		Fields: []Field{
			{Name: "S3_BUCKETNAME", Prompt: "S3 bucket name"},
			{Name: "S3_ENDPOINT", Prompt: "S3 endpoint (host[:port])"},
			{Name: "S3_REGION", Optional: true, Default: "none", Prompt: "S3 region"},
			{Name: "S3_PATH", Optional: true, Prompt: "Path inside the bucket"},
			{Name: "S3_ID", Secret: true, Key: "s3_id", BreakGlass: true, Prompt: "S3 access key ID"},
			{Name: "S3_SECRET", Secret: true, Key: "s3_secret", BreakGlass: true, Prompt: "S3 secret key"},
		},
		URL: func(v Values) string {
			return fmt.Sprintf("%s://%s@%s/%s", scheme, v["S3_REGION"], v["S3_ENDPOINT"], sub(v["S3_BUCKETNAME"], v["S3_PATH"]))
		},
		CheckInterval: day,
		Remote: func(v Values) (map[string]string, string) {
			region := v["S3_REGION"]
			// duplicacy's "none" (region-less endpoints such as MinIO); SigV4 needs some region.
			if region == "" || region == "none" {
				region = "us-east-1"
			}
			endpoint := "https://" + v["S3_ENDPOINT"]
			if !tls {
				endpoint = "http://" + v["S3_ENDPOINT"]
			}
			return map[string]string{
				"TYPE": "s3", "PROVIDER": provider, "ACCESS_KEY_ID": v["S3_ID"], "SECRET_ACCESS_KEY": v["S3_SECRET"],
				// Path-style addressing works everywhere, self-hosted endpoints included.
				"ENDPOINT": endpoint, "REGION": region, "FORCE_PATH_STYLE": "true",
				// A key limited to its bucket may not create or list buckets.
				"NO_CHECK_BUCKET": "true",
				// s3c is Duplicacy's type for providers that require V2 signing.
				"V2_AUTH": fmt.Sprint(scheme == "s3c"),
			}, sub(v["S3_BUCKETNAME"], v["S3_PATH"])
		},
	}
}

func sftpType(scheme string) Type {
	return Type{
		Name: scheme,
		Fields: []Field{
			{Name: "SFTP_URL", Prompt: "SFTP host (IP or FQDN)"},
			{Name: "SFTP_PORT", Default: "22", Prompt: "SFTP port"},
			{Name: "SFTP_USER", Prompt: "SFTP user"},
			{Name: "SFTP_PATH", Prompt: "SFTP path"},
		},
		URL: func(v Values) string {
			return fmt.Sprintf("%s://%s@%s:%s//%s", scheme, v["SFTP_USER"], v["SFTP_URL"], v["SFTP_PORT"], v["SFTP_PATH"])
		},
		CheckInterval: 7 * day,
		// The SSH key, known hosts and SFTP options are the kit's to add (internal/kit).
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "sftp", "HOST": v["SFTP_URL"], "PORT": v["SFTP_PORT"], "USER": v["SFTP_USER"]}, "/" + v["SFTP_PATH"]
		},
	}
}

func b2Fields(extra ...Field) []Field {
	return append(extra,
		Field{Name: "B2_BUCKETNAME", Prompt: "B2 bucket name"},
		Field{Name: "B2_PATH", Optional: true, Prompt: "Path inside the bucket"},
		Field{Name: "B2_ID", Secret: true, Key: "b2_id", BreakGlass: true, Prompt: "B2 key ID"},
		Field{Name: "B2_KEY", Secret: true, Key: "b2_key", BreakGlass: true, Prompt: "B2 application key"},
	)
}

func b2Remote(v Values) (map[string]string, string) {
	r := map[string]string{"TYPE": "b2", "ACCOUNT": v["B2_ID"], "KEY": v["B2_KEY"]}
	if host := v["B2_DOWNLOAD_HOST"]; host != "" {
		r["DOWNLOAD_URL"] = "https://" + host
	}
	return r, sub(v["B2_BUCKETNAME"], v["B2_PATH"])
}

// oneDrive is OneDrive personal ("one") or business ("odb"), with the user's own app (no
// duplicacy.com refresh, ADR 26). Duplicacy rewrites the token file as it refreshes, so the
// token is passed from a writable copy (internal/config prepares it).
func oneDrive(scheme, prefix string) Type {
	fields := []Field{{Name: prefix + "_PATH", Prompt: "Path in the drive"}}
	// Duplicacy honors a drive ID only for business drives; a personal one is always the user's own.
	if scheme == "odb" {
		fields = []Field{
			{Name: prefix + "_PATH", Optional: true, Prompt: "Path in the drive"},
			{Name: prefix + "_DRIVE_ID", Optional: true, Prompt: "Drive ID (empty for your own drive)"},
		}
	}
	fields = append(fields,
		Field{Name: prefix + "_CLIENT_ID", Key: scheme + "_client_id", Prompt: "Client ID of your app registration"},
		Field{Name: prefix + "_CLIENT_SECRET", Secret: true, Key: scheme + "_client_secret", Prompt: "Client secret of your app registration"},
		Field{Name: prefix + "_TOKEN", Secret: true, Key: scheme + "_token", Path: true, Rotates: true, Prompt: "Token file"},
	)
	return Type{
		Name:    scheme,
		Fields:  fields,
		Require: func(v Values) string { return requirePath(v, prefix+"_PATH", prefix+"_DRIVE_ID") },
		URL: func(v Values) string {
			drive := v[prefix+"_DRIVE_ID"]
			if drive != "" {
				drive += "@"
			}
			return fmt.Sprintf("%s://%s%s", scheme, drive, strings.Trim(v[prefix+"_PATH"], "/"))
		},
		CheckInterval: 7 * day,
		// The token, drive ID and drive type (which rclone requires) are the kit's to add.
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "onedrive", "CLIENT_ID": v[prefix+"_CLIENT_ID"], "CLIENT_SECRET": v[prefix+"_CLIENT_SECRET"]},
				"/" + strings.Trim(v[prefix+"_PATH"], "/")
		},
	}
}

// requirePath asks for a drive path when no drive is named: Duplicacy cannot parse a URL with
// nothing after "://" (a named drive's root is "drive@").
func requirePath(v Values, path, drive string) string {
	if strings.Trim(v[path], "/") == "" && (drive == "" || v[drive] == "") {
		return path
	}
	return ""
}

// Types are every storage type a configuration may use, by name. Each one's settings,
// secrets, Duplicacy URL and credentials, kit remote and check interval live here; the
// existing four keep exactly the variables and URLs they always had (ADR 1).
var Types = map[string]Type{}

// StorageTypes are the type names, in a stable order (for messages and init).
var StorageTypes []string

func register(t Type) { Types[t.Name] = t; StorageTypes = append(StorageTypes, t.Name) }

func init() {
	register(Type{
		Name:          "local",
		Fields:        []Field{{Name: "LOCAL_PATH", Prompt: "Local path"}},
		URL:           func(v Values) string { return v["LOCAL_PATH"] },
		CheckInterval: day,
		Remote:        func(v Values) (map[string]string, string) { return map[string]string{"TYPE": "local"}, v["LOCAL_PATH"] },
	})
	register(sftpType("sftp"))
	register(sftpType("sftpc"))
	register(Type{
		Name:          "b2",
		Fields:        b2Fields(),
		URL:           func(v Values) string { return "b2://" + sub(v["B2_BUCKETNAME"], v["B2_PATH"]) },
		CheckInterval: day,
		Remote:        b2Remote,
	})
	register(Type{
		Name:   "b2-custom",
		Fields: b2Fields(Field{Name: "B2_DOWNLOAD_HOST", Prompt: "B2 download host"}),
		URL: func(v Values) string {
			return "b2-custom://" + v["B2_DOWNLOAD_HOST"] + "/" + sub(v["B2_BUCKETNAME"], v["B2_PATH"])
		},
		CheckInterval: day,
		Remote:        b2Remote,
	})
	register(s3Type("s3", true, "Other"))
	register(s3Type("s3c", true, "Other"))
	register(s3Type("minio", false, "Minio"))
	register(s3Type("minios", true, "Minio"))
	register(Type{
		Name: "wasabi",
		Fields: []Field{
			{Name: "WASABI_BUCKETNAME", Prompt: "Wasabi bucket name"},
			{Name: "WASABI_ENDPOINT", Default: "s3.wasabisys.com", Prompt: "Wasabi endpoint"},
			{Name: "WASABI_REGION", Default: "us-east-1", Prompt: "Wasabi region"},
			{Name: "WASABI_PATH", Optional: true, Prompt: "Path inside the bucket"},
			{Name: "WASABI_KEY", Secret: true, Key: "wasabi_key", BreakGlass: true, Prompt: "Wasabi access key"},
			{Name: "WASABI_SECRET", Secret: true, Key: "wasabi_secret", BreakGlass: true, Prompt: "Wasabi secret key"},
		},
		URL: func(v Values) string {
			return fmt.Sprintf("wasabi://%s@%s/%s", v["WASABI_REGION"], v["WASABI_ENDPOINT"], sub(v["WASABI_BUCKETNAME"], v["WASABI_PATH"]))
		},
		CheckInterval: day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "s3", "PROVIDER": "Wasabi", "ACCESS_KEY_ID": v["WASABI_KEY"], "SECRET_ACCESS_KEY": v["WASABI_SECRET"],
				"ENDPOINT": "https://" + v["WASABI_ENDPOINT"], "REGION": v["WASABI_REGION"], "NO_CHECK_BUCKET": "true"}, sub(v["WASABI_BUCKETNAME"], v["WASABI_PATH"])
		},
	})
	register(Type{
		Name: "azure",
		Fields: []Field{
			{Name: "AZURE_ACCOUNT", Prompt: "Azure storage account"},
			{Name: "AZURE_CONTAINER", Prompt: "Azure container"},
			{Name: "AZURE_KEY", Secret: true, Key: "azure_key", BreakGlass: true, Prompt: "Azure access key"},
		},
		URL:           func(v Values) string { return "azure://" + v["AZURE_ACCOUNT"] + "/" + v["AZURE_CONTAINER"] },
		CheckInterval: day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "azureblob", "ACCOUNT": v["AZURE_ACCOUNT"], "KEY": v["AZURE_KEY"]}, v["AZURE_CONTAINER"]
		},
	})
	register(Type{
		Name: "gcs",
		Fields: []Field{
			{Name: "GCS_BUCKETNAME", Prompt: "GCS bucket name"},
			{Name: "GCS_PATH", Optional: true, Prompt: "Path inside the bucket"},
			{Name: "GCS_TOKEN", Secret: true, Key: "gcs_token", Path: true, Prompt: "Service account (JSON) file"},
		},
		URL:           func(v Values) string { return "gcs://" + sub(v["GCS_BUCKETNAME"], v["GCS_PATH"]) },
		CheckInterval: day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "google cloud storage", "SERVICE_ACCOUNT_CREDENTIALS": v["GCS_TOKEN"], "BUCKET_POLICY_ONLY": "true"},
				sub(v["GCS_BUCKETNAME"], v["GCS_PATH"])
		},
	})
	register(Type{
		Name: "gcd",
		Fields: []Field{
			{Name: "GCD_PATH", Optional: true, Prompt: "Path in the drive"},
			{Name: "GCD_DRIVE", Optional: true, Prompt: "Shared drive ID, from its URL (empty for My Drive)"},
			{Name: "GCD_TOKEN", Secret: true, Key: "gcd_token", Path: true, Prompt: "Token file from your own OAuth app, or a service account file"},
		},
		Require: func(v Values) string { return requirePath(v, "GCD_PATH", "GCD_DRIVE") },
		URL: func(v Values) string {
			drive := v["GCD_DRIVE"]
			if drive != "" {
				drive += "@"
			}
			return "gcd://" + drive + strings.Trim(v["GCD_PATH"], "/")
		},
		CheckInterval: 7 * day,
		Remote: func(v Values) (map[string]string, string) {
			r := googleDrive(v["GCD_TOKEN"])
			r["TYPE"], r["TEAM_DRIVE"] = "drive", v["GCD_DRIVE"]
			return r, "/" + strings.Trim(v["GCD_PATH"], "/")
		},
	})
	register(oneDrive("one", "ONE"))
	register(oneDrive("odb", "ODB"))
	register(Type{
		Name: "dropbox",
		Fields: []Field{
			{Name: "DROPBOX_PATH", Prompt: "Folder in the Dropbox (inside the app's folder for an app-folder app)"},
			{Name: "DROPBOX_APP_KEY", Key: "dropbox_client_id", Prompt: "App key of your Dropbox app"},
			{Name: "DROPBOX_APP_SECRET", Secret: true, Key: "dropbox_client_secret", Prompt: "App secret of your Dropbox app"},
			{Name: "DROPBOX_TOKEN", Secret: true, Key: "dropbox_token", Prompt: "Refresh token"},
		},
		// Duplicacy joins the URL's first path part and the rest without a slash, so the slash
		// after the first part is doubled to keep the directory as written.
		URL: func(v Values) string {
			first, rest, nested := strings.Cut(strings.Trim(v["DROPBOX_PATH"], "/"), "/")
			if nested {
				return "dropbox://" + first + "//" + rest
			}
			return "dropbox://" + first
		},
		Require:       func(v Values) string { return requirePath(v, "DROPBOX_PATH", "") },
		CheckInterval: 7 * day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "dropbox", "CLIENT_ID": v["DROPBOX_APP_KEY"], "CLIENT_SECRET": v["DROPBOX_APP_SECRET"],
				// Relative, as Duplicacy's path is: rclone sends an absolute one with a path-root
				// header, which Dropbox refuses for an app-folder app.
				"TOKEN": `{"refresh_token":"` + v["DROPBOX_TOKEN"] + `","expiry":"2000-01-01T00:00:00Z"}`}, strings.Trim(v["DROPBOX_PATH"], "/")
		},
	})
	register(Type{
		Name: "swift",
		Fields: []Field{
			// Duplicacy's own form: user@auth-host/vN/container[/path][?arguments].
			{Name: "SWIFT_URL", Prompt: "Swift URL (user@auth-host/v3/container[/path][?domain=…&tenant=…])"},
			{Name: "SWIFT_KEY", Secret: true, Key: "swift_key", BreakGlass: true, Prompt: "Swift key (password)"},
		},
		URL:           func(v Values) string { return "swift://" + v["SWIFT_URL"] },
		CheckInterval: day,
		Remote:        swiftRemote,
	})
	for _, scheme := range []string{"webdav", "webdav-http"} {
		https := scheme == "webdav"
		register(Type{
			Name: scheme,
			Fields: []Field{
				{Name: "WEBDAV_HOST", Prompt: "WebDAV host[:port]"},
				{Name: "WEBDAV_USER", Prompt: "WebDAV user"},
				{Name: "WEBDAV_PATH", Prompt: "WebDAV path"},
				{Name: "WEBDAV_PASSWORD", Secret: true, Key: "webdav_password", BreakGlass: true, Prompt: "WebDAV password"},
			},
			URL: func(v Values) string {
				return fmt.Sprintf("%s://%s@%s/%s", scheme, v["WEBDAV_USER"], v["WEBDAV_HOST"], strings.Trim(v["WEBDAV_PATH"], "/"))
			},
			CheckInterval: 7 * day,
			Remote: func(v Values) (map[string]string, string) {
				base := "https://"
				if !https {
					base = "http://"
				}
				// PASS is obscured by the kit (rclone's format), never passed as is.
				return map[string]string{"TYPE": "webdav", "URL": base + v["WEBDAV_HOST"], "USER": v["WEBDAV_USER"], "PASS": v["WEBDAV_PASSWORD"]},
					"/" + strings.Trim(v["WEBDAV_PATH"], "/")
			},
		})
	}
	register(Type{
		Name: "smb",
		Fields: []Field{
			{Name: "SMB_HOST", Prompt: "SMB host[:port]"},
			{Name: "SMB_USER", Prompt: "SMB user"},
			{Name: "SMB_SHARE", Prompt: "SMB share"},
			{Name: "SMB_PATH", Optional: true, Prompt: "Path inside the share"},
			{Name: "SMB_PASSWORD", Secret: true, Key: "smb_password", BreakGlass: true, Prompt: "SMB password"},
		},
		URL: func(v Values) string {
			// Duplicacy needs the slash after the share even for the share's root.
			return fmt.Sprintf("smb://%s@%s/%s/%s", v["SMB_USER"], v["SMB_HOST"], v["SMB_SHARE"], strings.Trim(v["SMB_PATH"], "/"))
		},
		CheckInterval: 7 * day,
		Remote: func(v Values) (map[string]string, string) {
			host, port, _ := strings.Cut(v["SMB_HOST"], ":")
			r := map[string]string{"TYPE": "smb", "HOST": host, "USER": v["SMB_USER"], "PASS": v["SMB_PASSWORD"]}
			if port != "" {
				r["PORT"] = port
			}
			return r, sub(v["SMB_SHARE"], v["SMB_PATH"])
		},
	})
	register(Type{
		Name: "storj",
		Fields: []Field{
			{Name: "STORJ_SATELLITE", Prompt: "Storj satellite (host:port)"},
			{Name: "STORJ_BUCKET", Prompt: "Storj bucket"},
			{Name: "STORJ_PATH", Optional: true, Prompt: "Path inside the bucket"},
			{Name: "STORJ_KEY", Secret: true, Key: "storj_key", BreakGlass: true, Prompt: "Storj API key"},
			{Name: "STORJ_PASSPHRASE", Secret: true, Key: "storj_passphrase", Prompt: "Storj encryption passphrase"},
		},
		URL: func(v Values) string {
			return "storj://" + v["STORJ_SATELLITE"] + "/" + sub(v["STORJ_BUCKET"], v["STORJ_PATH"])
		},
		CheckInterval: day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "storj", "PROVIDER": "new", "SATELLITE_ADDRESS": v["STORJ_SATELLITE"],
				"API_KEY": v["STORJ_KEY"], "PASSPHRASE": v["STORJ_PASSPHRASE"]}, sub(v["STORJ_BUCKET"], v["STORJ_PATH"])
		},
	})
	register(Type{
		Name: "fabric",
		Fields: []Field{
			{Name: "FABRIC_ENDPOINT", Prompt: "File Fabric host"},
			{Name: "FABRIC_PATH", Optional: true, Prompt: "Path"},
			{Name: "FABRIC_TOKEN", Secret: true, Key: "fabric_token", BreakGlass: true, Prompt: "File Fabric permanent token"},
		},
		URL: func(v Values) string {
			return "fabric://" + v["FABRIC_ENDPOINT"] + "/" + strings.Trim(v["FABRIC_PATH"], "/")
		},
		CheckInterval: 7 * day,
		Remote: func(v Values) (map[string]string, string) {
			return map[string]string{"TYPE": "filefabric", "URL": "https://" + v["FABRIC_ENDPOINT"], "PERMANENT_TOKEN": v["FABRIC_TOKEN"]},
				"/" + strings.Trim(v["FABRIC_PATH"], "/")
		},
	})
}

// googleDrive is rclone's Google Drive credentials from Duplicacy's token file: a service
// account as it is, else the client of the user's own OAuth app and its token.
func googleDrive(file string) map[string]string {
	var f struct {
		Type         string          `json:"type"`
		ClientID     string          `json:"client_id"`
		ClientSecret string          `json:"client_secret"`
		Token        json.RawMessage `json:"token"`
	}
	if json.Unmarshal([]byte(file), &f) != nil || f.Type == "service_account" {
		return map[string]string{"SERVICE_ACCOUNT_CREDENTIALS": file}
	}
	return map[string]string{"CLIENT_ID": f.ClientID, "CLIENT_SECRET": f.ClientSecret, "TOKEN": string(f.Token)}
}

// swiftRemote reads Duplicacy's Swift URL (user@auth-host/vN/container[/path][?args]) into
// rclone's settings: the auth URL keeps its version, and domain, tenant and region map across.
func swiftRemote(v Values) (map[string]string, string) {
	u, args, _ := strings.Cut(v["SWIFT_URL"], "?")
	r := map[string]string{"TYPE": "swift", "KEY": v["SWIFT_KEY"]}
	// Split as Duplicacy does: each version marker in turn, the last one found winning.
	dir := ""
	for _, ver := range []string{"/v1/", "/v1.0/", "/v2/", "/v2.0/", "/v3/", "/v3.0/", "/v4/", "/v4.0/"} {
		if i := strings.Index(u, ver); i >= 0 {
			u, dir = u[:i+len(ver)-1], u[i+len(ver):]
		}
	}
	if at := strings.LastIndex(u, "@"); at > 0 {
		r["USER"], u = u[:at], u[at+1:]
	}
	protocol := "https"
	keys := map[string]string{"domain": "DOMAIN", "user": "USER", "user_id": "USER_ID", "tenant": "TENANT", "tenant_id": "TENANT_ID",
		"tenant_domain": "TENANT_DOMAIN", "region": "REGION", "auth_version": "AUTH_VERSION",
		// Duplicacy's own spelling of endpoint_type.
		"endpiont_type": "ENDPOINT_TYPE"}
	for _, pair := range strings.Split(args, "&") {
		k, val, _ := strings.Cut(pair, "=")
		if k == "protocol" {
			protocol = val
		} else if name, ok := keys[k]; ok {
			r[name] = val
		}
	}
	r["AUTH"] = protocol + "://" + u
	if dir == "" {
		for _, pair := range strings.Split(args, "&") {
			if k, val, _ := strings.Cut(pair, "="); k == "storage_dir" {
				dir = val
			}
		}
	}
	return r, strings.Trim(dir, "/")
}
