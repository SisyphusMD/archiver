// Package logging writes a pipeline's log: one line per
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
	Dir      string // /opt/archiver/logs
	Basename string // "archiver" or "maintenance"
	Stdout   io.Writer
	// Unwritable is told that the log cannot be written (at the first failure, then hourly
	// while it lasts), and Writable at the
	// first line written and the first after a failure, so the incident can clear. Error
	// lines do not notify one by one: a run notifies once with its Summary (ADR 36).
	Unwritable func(message string)
	Writable   func()
	Now        func() time.Time

	mu           sync.Mutex
	errors       int
	messages     []string // the first error lines, for the run's notification
	reportable   int      // every error line a notification may carry
	writeFailed  bool
	wrote        bool
	lastRaised   time.Time // when Unwritable was last told
	lastWritable time.Time // when Writable was last told
}

// unwritableEvery is how often a log that keeps failing tells Unwritable again (and one
// that keeps working, Writable), so the incident can repeat and retry missed destinations:
// not every line, since with the logs volume full the incident state cannot be written
// either, and nothing would deduplicate.
const unwritableEvery = time.Hour

func (l *Log) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Path is the log file messages are appended to.
func (l *Log) Path() string { return filepath.Join(l.Dir, l.Basename+".log") }

// Reportable is how many of the run's own errors were logged: those a notification
// carries, not a notification's own failure or errors counted from elsewhere.
func (l *Log) Reportable() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reportable
}

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

// Message logs one line for service ("archiver" when empty). An error is counted and kept
// for the run's notification; a line that cannot be written is reported on stdout and
// counted, never fatal.
func (l *Log) Message(level, service, msg string) { l.message(level, service, msg, true) }

// Unnotified logs like Message but leaves the line out of the run's notification: for the
// outcome of a notification, whose failure must not set off another one (which would fail
// too, and loop).
func (l *Log) Unnotified(level, service, msg string) { l.message(level, service, msg, false) }

// maxMessages bounds the error lines a notification carries; the log has them all.
const maxMessages = 10

// Summary is the run's errors for its one notification: the first lines and how many
// more the log holds.
func (l *Log) Summary() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	for _, m := range l.messages {
		b.WriteString(m + "\n")
	}
	if more := l.reportable - len(l.messages); more > 0 {
		fmt.Fprintf(&b, "... and %d more in %s.\n", more, l.Basename+".log")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (l *Log) message(level, service, msg string, notify bool) {
	if service == "" {
		service = "archiver"
	}
	line := fmt.Sprintf("[%s] [%s] [Service: %s] %s", l.now().Format("2006-01-02 15:04:05"), level, service, msg)
	l.mu.Lock()
	err := appendLine(l.Path(), line)
	if level == Error {
		l.errors++
		if notify {
			l.reportable++
			if len(l.messages) < maxMessages {
				l.messages = append(l.messages, fmt.Sprintf("[%s] %s", service, msg))
			}
		}
	}
	// Only a line that may notify claims the one notification, so a failure first met while
	// recording a notification's outcome still gets said by the next line.
	firstFailure := err != nil && notify && (!l.writeFailed || l.now().Sub(l.lastRaised) >= unwritableEvery)
	if err != nil {
		l.errors++
	}
	if firstFailure {
		l.lastRaised = l.now()
	}
	if firstFailure {
		l.writeFailed = true
	}
	// Writable: the first line of the run, the first after a failure, and hourly after
	// that, so a recovery notice an outage kept back is sent while writes keep working.
	writable := err == nil && notify && l.Writable != nil && (!l.wrote || l.writeFailed || l.now().Sub(l.lastWritable) >= unwritableEvery)
	if writable {
		l.wrote, l.writeFailed, l.lastWritable = true, false, l.now()
	}
	l.mu.Unlock()
	if err != nil && l.Stdout != nil {
		fmt.Fprintf(l.Stdout, "[%s] [ERROR] Failed to log message for %s service to %s. Check if the log file is writable and disk space is available.\n",
			l.now().Format("2006-01-02 15:04:05"), service, l.Path())
	}
	if (level == Warning || level == Error) && l.Stdout != nil {
		fmt.Fprintln(l.Stdout, line)
	}
	// An unwritable log (a full logs volume) is a failure in itself, said once a run: every
	// later line would fail the same way.
	if writable && l.Writable != nil {
		l.Writable()
	}
	if firstFailure && l.Unwritable != nil {
		l.Unwritable(fmt.Sprintf("Cannot write %s (%v); check its volume's free space. The run's messages go to stdout only.", l.Path(), err))
	}
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
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
		_, _ = io.Copy(io.Discard, pr)
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
	if err := os.MkdirAll(old, 0o755); err != nil { //nolint:gosec // G301: a directory holding nothing secret
		l.Message(Error, "", "Unable to create log directory "+old+".")
	}
	// Each run gets a file of its own, even when two start within the same second: a run's
	// log must not carry an earlier run's errors.
	stamp := l.Basename + "-" + l.now().Format("2006-01-02_150405")
	name, file := "", ""
	for i := 0; ; i++ {
		name = stamp + ".log"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.log", stamp, i)
		}
		file = filepath.Join(old, name)
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
		if err == nil {
			f.Close()
			break
		}
		if !os.IsExist(err) || i > 100 {
			l.Message(Error, "", "Could not create log file "+file+".")
			break
		}
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
