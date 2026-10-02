// Package logging writes a pipeline's log the way lib/core/logging.sh does: one line per
// message in archiver.log (a symlink to the current run's file under prior_logs), warnings
// and errors echoed to stdout, and every error counted and sent to the notifier. status,
// healthcheck, and logs read these files, so the line format is load-bearing.
package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Levels, as written in the log.
const (
	Info    = "INFO"
	Warning = "WARNING"
	Error   = "ERROR"
)

// Log is one pipeline's log. It is safe for concurrent use.
type Log struct {
	Dir        string // /opt/archiver/logs
	Basename   string // "archiver" or "maintenance"
	ErrorTitle string // notification title for an error, e.g. "Backup Error"
	Stdout     io.Writer
	Notify     func(title, message string)
	Now        func() time.Time

	mu     sync.Mutex
	errors int
}

func (l *Log) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Path is the log file messages are appended to.
func (l *Log) Path() string { return filepath.Join(l.Dir, l.Basename+".log") }

// Errors is how many errors have been logged.
func (l *Log) Errors() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.errors
}

// AddErrors counts errors that were reported elsewhere (a helper that logged and notified
// them itself), so they still decide the run's result.
func (l *Log) AddErrors(n int) {
	l.mu.Lock()
	l.errors += n
	l.mu.Unlock()
}

// Message logs one line for service ("archiver" when empty). An error is counted and
// notified; a line that cannot be written is reported on stdout and counted, never fatal.
func (l *Log) Message(level, service, msg string) {
	if service == "" {
		service = "archiver"
	}
	line := fmt.Sprintf("[%s] [%s] [Service: %s] %s", l.now().Format("2006-01-02 15:04:05"), level, service, msg)
	l.mu.Lock()
	err := appendLine(l.Path(), line)
	if level == Error {
		l.errors++
	}
	if err != nil {
		l.errors++
	}
	l.mu.Unlock()
	if err != nil && l.Stdout != nil {
		fmt.Fprintf(l.Stdout, "[%s] [ERROR] Failed to log message for %s service to %s. Check if the log file is writable and disk space is available.\n",
			l.now().Format("2006-01-02 15:04:05"), service, l.Path())
	}
	if (level == Warning || level == Error) && l.Stdout != nil {
		fmt.Fprintln(l.Stdout, line)
	}
	if level == Error && l.Notify != nil {
		l.Notify(l.ErrorTitle, fmt.Sprintf("[%s] %s", service, msg))
	}
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(line + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// Lines logs each line r yields until EOF, as log_output does for a command's output.
func (l *Log) Lines(level, service string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l.Message(level, service, sc.Text())
	}
	if err := sc.Err(); err != nil {
		l.Message(Warning, service, "Output not logged after this point: "+err.Error())
	}
}

// Prefixed is the level a hook's output line asks for: a line starting "[ERROR] " or
// "[WARNING] " is logged at that level (an error counted and notified) without the prefix,
// and any other line at Info.
func Prefixed(line string) (level, msg string) {
	for _, lv := range []string{Error, Warning} {
		if rest, ok := strings.CutPrefix(line, "["+lv+"] "); ok {
			return lv, rest
		}
	}
	return Info, line
}

// HookLines logs a hook's output, each line at the level its prefix asks for.
func (l *Log) HookLines(service string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		level, msg := Prefixed(sc.Text())
		l.Message(level, service, msg)
	}
	if err := sc.Err(); err != nil {
		l.Message(Warning, service, "Output not logged after this point: "+err.Error())
	}
}

// Writer is an io.Writer that logs each complete line written to it. Close flushes a final
// partial line.
func (l *Log) Writer(level, service string) io.WriteCloser {
	return l.writer(func(r io.Reader) { l.Lines(level, service, r) })
}

// HookWriter is Writer for a hook's output (HookLines).
func (l *Log) HookWriter(service string) io.WriteCloser {
	return l.writer(func(r io.Reader) { l.HookLines(service, r) })
}

func (l *Log) writer(consume func(io.Reader)) io.WriteCloser {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		consume(pr)
		io.Copy(io.Discard, pr)
		close(done)
	}()
	return &lineWriter{pw: pw, done: done}
}

type lineWriter struct {
	pw   *io.PipeWriter
	done chan struct{}
}

func (w *lineWriter) Write(p []byte) (int, error) { return w.pw.Write(p) }
func (w *lineWriter) Close() error {
	err := w.pw.Close()
	<-w.done
	return err
}

// Rotate starts a fresh file for this run under prior_logs, points the log symlink at it,
// and deletes files there older than seven days.
func (l *Log) Rotate() {
	old := filepath.Join(l.Dir, "prior_logs")
	if err := os.MkdirAll(old, 0o755); err != nil {
		l.Message(Error, "", "Unable to create log directory "+old+".")
	}
	name := l.Basename + "-" + l.now().Format("2006-01-02_150405") + ".log"
	file := filepath.Join(old, name)
	if f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0o644); err != nil {
		l.Message(Error, "", "Could not create log file "+file+".")
	} else {
		f.Close()
	}
	l.Message(Info, "", "Log file created: "+file+".")
	link := l.Path()
	tmp := link + ".tmp"
	os.Remove(tmp)
	if err := os.Symlink(filepath.Join("prior_logs", name), tmp); err == nil {
		err = os.Rename(tmp, link)
		if err != nil {
			l.Message(Error, "", fmt.Sprintf("Could not update/create symlink for '%s.log' to %s.", l.Basename, file))
		}
	} else {
		l.Message(Error, "", fmt.Sprintf("Could not update/create symlink for '%s.log' to %s.", l.Basename, file))
	}
	l.Message(Info, "", fmt.Sprintf("Symlink '%s.log' updated to point to %s.", l.Basename, file))
	// find -mtime +7: older than eight whole days.
	cutoff := l.now().Add(-8 * 24 * time.Hour)
	entries, err := os.ReadDir(old)
	if err != nil {
		l.Message(Error, "", "Failed to delete old 'archiver' log files.")
		return
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(old, e.Name()))
		}
	}
	l.Message(Info, "", "Old log files deleted (>7 days).")
}

// Duration formats seconds as format_duration does: "2 days, 3 hours, and 15 minutes".
func Duration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	units := []struct {
		n    int64
		name string
	}{
		{seconds / 86400, "day"}, {seconds % 86400 / 3600, "hour"}, {seconds % 3600 / 60, "minute"}, {seconds % 60, "second"},
	}
	var parts []string
	for _, u := range units {
		if u.n > 0 {
			s := fmt.Sprintf("%d %s", u.n, u.name)
			if u.n != 1 {
				s += "s"
			}
			parts = append(parts, s)
		}
	}
	switch len(parts) {
	case 0:
		return "0 seconds"
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

// Timestamp formats a Unix time as format_timestamp does.
func Timestamp(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}
