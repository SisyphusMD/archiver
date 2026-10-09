package entrypoint

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"time"
)

// logLine is a line as logging.Log writes it: "[time] [LEVEL] [Service: name] message".
var logLine = regexp.MustCompile(`^\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)\] \[([A-Z]+)\] \[Service: ([^\]]*)\] (.*)$`)

// JSONLines returns a writer that turns each complete line written to it into one JSON
// object on w (LOG_FORMAT=json): time, level, service, log (which log it came from) and
// msg, so a collector can index them without parsing the text. A line in another form
// (a banner, a program's own output) keeps its text as msg. Each Write reaches w in one
// call, so writers sharing w never interleave mid-line.
func JSONLines(log string, w io.Writer) io.Writer { return JSONLinesAt(log, "INFO", w) }

// JSONLinesAt is JSONLines with level for lines in another form (ERROR for a stderr).
func JSONLinesAt(log, level string, w io.Writer) io.Writer {
	return &jsonWriter{log: log, level: level, w: w, now: time.Now}
}

type jsonWriter struct {
	log     string
	level   string // of a line in another form
	w       io.Writer
	now     func() time.Time
	partial []byte
}

type jsonEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Service string `json:"service,omitempty"`
	Log     string `json:"log"`
	Msg     string `json:"msg"`
}

func (j *jsonWriter) Write(p []byte) (int, error) {
	data := append(j.partial, p...)
	var out bytes.Buffer
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		line := string(data[:i])
		data = data[i+1:]
		e := jsonEntry{Level: j.level, Log: j.log, Msg: line}
		if m := logLine.FindStringSubmatch(line); m != nil {
			e.Level, e.Service, e.Msg = m[2], m[3], m[4]
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local); err == nil {
				e.Time = t.Format(time.RFC3339)
			}
		}
		if e.Time == "" {
			e.Time = j.now().Format(time.RFC3339)
		}
		b, _ := json.Marshal(e)
		out.Write(b)
		out.WriteByte('\n')
	}
	j.partial = append([]byte(nil), data...)
	if out.Len() > 0 {
		if _, err := j.w.Write(out.Bytes()); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
