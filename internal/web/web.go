// Package web is the read-only status page (ADR 38): backup health, each service's last
// backup, the copy workers, maintenance, drills, open incidents and a log's last lines. It
// shows what `archiver status --json` holds, changes nothing, and has no login of its own:
// it is meant to sit behind a reverse proxy with authentication.
package web

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/status"
)

// Logs are the logs the page can show, by name.
var Logs = []string{"archiver", "maintenance", "copies", "drill"}

// logLines is how many of a log's last lines the page shows.
const logLines = 200

// Serve serves the page on port until ctx ends.
func Serve(ctx context.Context, port string, l layout.Layout, getenv func(string) string) error {
	srv := &http.Server{Handler: Handler(l, getenv), ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler serves the page at / and the snapshot at /status.json; anything but GET or HEAD
// is refused, since nothing here changes anything.
func Handler(l layout.Layout, getenv func(string) string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		name := r.URL.Query().Get("log")
		if !contains(Logs, name) {
			name = "archiver"
		}
		s := status.Take(l, getenv, time.Now())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page.Execute(w, view{S: s, Now: time.Now(), Log: name, Logs: Logs, Lines: tail(filepath.Join(l.LogDir(), name+".log"), logLines)})
	})
	mux.HandleFunc("/status.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(status.Take(l, getenv, time.Now()))
	})
	return readOnly(mux)
}

func readOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "read-only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// tail is the last n lines of path, read from its end.
func tail(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const window = 512 << 10
	if fi, err := f.Stat(); err == nil && fi.Size() > window {
		f.Seek(-window, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = redact(l)
	}
	return lines
}

var (
	appriseKey  = regexp.MustCompile(`/(notify|api/push)/[^\s"'/?#]+`)
	urlUserinfo = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
)

// redact blanks what a log line may hold that the page must not show, whatever wrote it
// (older versions logged a failed notification's URL): an Apprise key or Uptime Kuma push
// token in a URL path, and credentials in any URL.
func redact(line string) string {
	line = appriseKey.ReplaceAllString(line, "/${1}/***")
	return urlUserinfo.ReplaceAllString(line, "${1}***@")
}

type view struct {
	S     status.Snapshot
	Now   time.Time
	Log   string
	Logs  []string
	Lines []string
}

func ago(epoch int64, now time.Time) string {
	if epoch == 0 {
		return "never"
	}
	return status.Age(epoch, now)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"ago":           ago,
	"base":          filepath.Base,
	"servicekeys":   func(s status.Snapshot) []string { return keys(s.Services) },
	"copykeys":      func(s status.Snapshot) []string { return keys(s.Copies) },
	"drillstorages": func(s status.Snapshot) []string { return keys(s.Drills) },
	"drillservices": func(s status.Snapshot, storage string) []string { return keys(s.Drills[storage]) },
	"incidentkeys":  func(s status.Snapshot) []string { return keys(s.Incidents) },
	"lower":         strings.ToLower,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="30"><title>Archiver status</title>
<style>
:root{--bg:#fff;--fg:#1d1f21;--muted:#666;--line:#ddd;--ok:#1a7f37;--degraded:#9a6700;--failing:#cf222e}
@media (prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#8b949e;--line:#30363d;--ok:#3fb950;--degraded:#d29922;--failing:#f85149}}
body{background:var(--bg);color:var(--fg);font:15px/1.45 system-ui,sans-serif;margin:0 auto;max-width:72rem;padding:1rem}
h1{font-size:1.4rem}h2{font-size:1.1rem;margin-top:1.6rem;border-bottom:1px solid var(--line);padding-bottom:.2rem}
table{border-collapse:collapse;width:100%;display:block;overflow-x:auto}td,th{text-align:left;padding:.25rem .6rem;border-bottom:1px solid var(--line);white-space:nowrap}
.ok{color:var(--ok)}.degraded{color:var(--degraded)}.failing,.bad{color:var(--failing)}.muted{color:var(--muted)}
.state{font-size:1.3rem;font-weight:700}pre{background:rgba(127,127,127,.1);padding:.6rem;overflow-x:auto;font-size:12.5px;max-height:32rem}
nav a{margin-right:.8rem}
</style></head><body>
<h1>Archiver</h1>
<p class="muted">Read-only. Updated {{.Now.Format "2006-01-02 15:04:05"}}; refreshes every 30 seconds. <a href="/status.json">JSON</a></p>
<h2>Backup health</h2>
<p class="state {{lower .S.BackupHealth.State}}">{{.S.BackupHealth.State}}</p>
{{with .S.BackupHealth.Reasons}}<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>{{end}}
<p>Backup: {{if .S.Backup.Running}}running{{if .S.Backup.Paused}}, paused{{end}} ({{.S.Backup.Stage}}){{else}}not running{{end}}.
Maintenance: {{if .S.Maintenance.Running}}running{{if .S.Maintenance.Paused}}, paused{{end}} ({{.S.Maintenance.Stage}}){{else}}not running{{end}}.
Restore drill: {{if .S.Drill.Running}}running{{if .S.Drill.Paused}}, paused{{end}}{{else}}not running{{end}}.</p>
<h2>Services</h2>
{{$s := .S}}{{$now := .Now}}
{{if servicekeys $s}}<table><tr><th>Service</th><th>Directory</th><th>Last backup</th><th>Result</th><th>Last good</th><th>Revision</th><th>Took</th></tr>
{{range servicekeys $s}}{{$r := index $s.Services .}}<tr><td>{{base .}}</td><td class="muted">{{.}}</td><td>{{ago $r.LastAttempt $now}}</td>
<td class="{{if eq $r.Result "success"}}ok{{else}}bad{{end}}">{{$r.Result}}</td><td>{{ago $r.LastSuccess $now}}</td><td>{{if $r.Revision}}{{$r.Revision}}{{end}}</td><td>{{$r.Seconds}}s</td></tr>{{end}}</table>
{{else}}<p class="muted">No backup has run yet.</p>{{end}}
<h2>Copies</h2>
{{if copykeys $s}}<table><tr><th>Secondary</th><th>Status</th><th>Behind</th><th>Last caught up</th><th>Last check</th><th>Last error</th></tr>
{{range copykeys $s}}{{$c := index $s.Copies .}}<tr><td>{{.}}</td><td class="{{if $c.FailingSince}}bad{{end}}">{{$c.Status}}{{if $c.Paused}}, paused{{end}}</td><td>{{$c.Behind}}</td>
<td>{{ago $c.LastSuccess $now}}</td><td>{{ago $c.LastCheck $now}}</td><td class="muted">{{$c.LastError}}</td></tr>{{end}}</table>
{{else}}<p class="muted">No copy workers (one storage, or no daemon).</p>{{end}}
<h2>Storage maintenance</h2>
{{if $s.Storages}}<table><tr><th>Storage</th><th>Last check</th><th>Last prune</th><th>Last exhaustive prune</th></tr>
{{range $s.Storages}}<tr><td>{{.Name}}</td><td>{{ago .Check $now}}</td><td>{{ago .Prune $now}}</td><td>{{ago .Exhaustive $now}}</td></tr>{{end}}</table>
{{else}}<p class="muted">Maintenance has not run yet.</p>{{end}}
<h2>Restore drills</h2>
{{if drillstorages $s}}<table><tr><th>Storage</th><th>Service</th><th>When</th><th>Result</th></tr>
{{range $st := drillstorages $s}}{{range drillservices $s $st}}{{$d := index (index $s.Drills $st) .}}<tr><td>{{$st}}</td><td>{{.}}</td><td>{{ago $d.At $now}}</td>
<td class="{{if $d.OK}}ok{{else if $d.Skipped}}muted{{else}}bad{{end}}">{{if $d.OK}}passed, revision {{$d.Revision}}{{else if $d.Skipped}}skipped: {{$d.Message}}{{else}}FAILED: {{$d.Message}}{{end}}</td></tr>{{end}}{{end}}</table>
{{else}}<p class="muted">No drills (RESTORE_DRILL_SCHEDULE unset, or none run yet).</p>{{end}}
<h2>Open incidents</h2>
{{if incidentkeys $s}}<ul>{{range incidentkeys $s}}<li>{{index $s.Incidents .}} <span class="muted">({{.}})</span></li>{{end}}</ul>{{else}}<p class="muted">None.</p>{{end}}
{{with $s.Envelope}}<p>{{.}}</p>{{end}}
<h2>Log</h2>
<nav>{{$cur := .Log}}{{range .Logs}}{{if eq . $cur}}<strong>{{.}}</strong>{{else}}<a href="/?log={{.}}">{{.}}</a>{{end}} {{end}}</nav>
<pre>{{range .Lines}}{{.}}
{{else}}(empty){{end}}</pre>
</body></html>`))
