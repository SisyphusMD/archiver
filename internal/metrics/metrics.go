// Package metrics renders what status knows as Prometheus metrics (ADR 35): a textfile in
// the logs volume for node-exporter's textfile collector, and the same on /metrics when
// METRICS_PORT asks for it.
package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/backuphealth"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/status"
)

type writer struct {
	b    strings.Builder
	seen map[string]bool
}

// gauge writes one sample, with its HELP and TYPE the first time the name appears.
func (w *writer) gauge(name, help string, value float64, labels ...string) {
	if !w.seen[name] {
		w.seen[name] = true
		fmt.Fprintf(&w.b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	}
	w.b.WriteString(name)
	if len(labels) > 0 {
		w.b.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				w.b.WriteByte(',')
			}
			fmt.Fprintf(&w.b, `%s="%s"`, labels[i], escape(labels[i+1]))
		}
		w.b.WriteByte('}')
	}
	fmt.Fprintf(&w.b, " %g\n", value)
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Render is the snapshot in Prometheus text format. Timestamps are Unix seconds; a time
// never reached is left out rather than written as 0.
func Render(s status.Snapshot) string {
	w := &writer{seen: map[string]bool{}}
	w.gauge("archiver_backup_health", "Backup health (ADR 32): 0 OK, 1 DEGRADED, 2 FAILING.", float64(backuphealth.Health{State: s.BackupHealth.State}.ExitCode()))
	for _, run := range []struct {
		name string
		r    status.Run
	}{{"backup", s.Backup}, {"maintenance", s.Maintenance}, {"drill", s.Drill}} {
		w.gauge("archiver_running", "Whether a run of this kind is in progress.", b2f(run.r.Running), "run", run.name)
	}
	for _, dir := range sortedKeys(s.Services) {
		r := s.Services[dir]
		l := []string{"service", filepath.Base(dir), "directory", dir}
		if r.LastSuccess != 0 {
			w.gauge("archiver_service_last_success_timestamp_seconds", "When the service last backed up to the primary.", float64(r.LastSuccess), l...)
		}
		w.gauge("archiver_service_last_attempt_timestamp_seconds", "When the service's last backup ended.", float64(r.LastAttempt), l...)
		w.gauge("archiver_service_last_duration_seconds", "How long the service's last backup took.", float64(r.Seconds), l...)
		w.gauge("archiver_service_last_ok", "Whether the service's last backup succeeded.", b2f(r.Result == "success"), l...)
		if r.Revision != 0 {
			w.gauge("archiver_service_last_revision", "The revision the service's last successful backup made.", float64(r.Revision), l...)
		}
		w.gauge("archiver_service_last_uploaded_bytes", "Bytes the service's last successful backup uploaded.", float64(r.Uploaded), l...)
	}
	for _, name := range sortedKeys(s.Copies) {
		c := s.Copies[name]
		l := []string{"target", name}
		w.gauge("archiver_copy_behind_revisions", "Revisions the secondary is behind the primary.", float64(c.Behind), l...)
		w.gauge("archiver_copy_failing", "Whether copies to the secondary are failing (retrying or down).", b2f(c.FailingSince != 0 || c.Status == copier.Retrying || c.Status == copier.Down), l...)
		w.gauge("archiver_copy_held", "Whether the secondary's copy worker waits for its copy window.", b2f(c.HeldUntil != 0), l...)
		if c.LastSuccess != 0 {
			w.gauge("archiver_copy_last_success_timestamp_seconds", "When the secondary last caught up.", float64(c.LastSuccess), l...)
		}
		if c.LastCheck != 0 {
			w.gauge("archiver_copy_last_check_timestamp_seconds", "When the secondary's check last passed.", float64(c.LastCheck), l...)
		}
	}
	for _, st := range s.Storages {
		l := []string{"storage", st.Name}
		if st.Check != 0 {
			w.gauge("archiver_storage_last_check_timestamp_seconds", "When maintenance last checked the storage successfully.", float64(st.Check), l...)
		}
		if st.Prune != 0 {
			w.gauge("archiver_storage_last_prune_timestamp_seconds", "When maintenance last pruned the storage successfully.", float64(st.Prune), l...)
		}
	}
	for _, storage := range sortedKeys(s.Drills) {
		for _, svc := range sortedKeys(s.Drills[storage]) {
			d := s.Drills[storage][svc]
			if d.Skipped {
				continue
			}
			l := []string{"storage", storage, "service", svc}
			w.gauge("archiver_drill_last_ok", "Whether the last restore drill of the service on the storage passed.", b2f(d.OK), l...)
			w.gauge("archiver_drill_last_timestamp_seconds", "When the last restore drill of the service on the storage ran.", float64(d.At), l...)
		}
	}
	w.gauge("archiver_incidents_open", "Open incidents (ADR 36): failures and problems not yet cleared.", float64(len(s.Incidents)))
	for _, k := range sortedKeys(s.Incidents) {
		w.gauge("archiver_incident_open", "An open incident, by key.", 1, "key", k, "title", s.Incidents[k])
	}
	return w.b.String()
}

// WriteFile writes the metrics to the textfile atomically, as node-exporter requires.
func WriteFile(l layout.Layout, getenv func(string) string, now time.Time) error {
	path := l.MetricsFile()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".archiver.prom-")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(Render(status.Take(l, getenv, now)))
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("writing %s: %v %v", path, werr, cerr)
	}
	_ = os.Chmod(tmp.Name(), 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	return os.Rename(tmp.Name(), path)
}

// Keep rewrites the textfile every interval until ctx ends: the copy workers' state
// changes between runs.
func Keep(ctx context.Context, l layout.Layout, getenv func(string) string, every time.Duration, logf func(error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := WriteFile(l, getenv, time.Now()); err != nil && logf != nil {
			logf(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Serve serves /metrics on port until ctx ends, rendered fresh for each request.
func Serve(ctx context.Context, port string, l layout.Layout, getenv func(string) string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, Render(status.Take(l, getenv, time.Now())))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
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
