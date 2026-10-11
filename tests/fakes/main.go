// Command fakes stands in for the cloud storage services no published emulator covers (ADR
// 47): each service's API, as far as Duplicacy and rclone call it, answered from files under
// one directory. The test's TLS front sends every service's real hostnames here and keeps the
// Host header, which picks the service.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	dataDir  string
	mu       sync.Mutex
	services = map[string]http.Handler{}
)

// register routes requests for these hostnames to h. Each service registers itself from init.
func register(h http.Handler, hosts ...string) {
	mu.Lock()
	defer mu.Unlock()
	for _, host := range hosts {
		if _, dup := services[host]; dup {
			log.Fatalf("host %s registered twice", host)
		}
		services[host] = h
	}
}

// recorder keeps the status for the request log.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func serve(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	mu.Lock()
	h := services[host]
	if h == nil {
		// Hosts made up per storage (B2's upload URLs, a CDN) register a suffix wildcard.
		for pattern, sh := range services {
			if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, pattern[1:]) {
				h = sh
				break
			}
		}
	}
	mu.Unlock()
	if h == nil {
		http.Error(rec, "no fake serves "+host, http.StatusNotFound)
	} else {
		h.ServeHTTP(rec, r)
	}
	log.Printf("%s %s %s -> %d", r.Method, host, r.URL.RequestURI(), rec.status)
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.StringVar(&dataDir, "data", "/data", "directory the services keep their files in")
	flag.Parse()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("serving %d hosts on %s", len(services), *addr)
	log.Fatal(http.ListenAndServe(*addr, http.HandlerFunc(serve)))
}
