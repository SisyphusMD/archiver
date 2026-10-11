package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// wasabi serves the one request Wasabi adds to S3: MOVE, a server-side rename that Duplicacy
// uses to turn chunks into fossils. The test front sends only MOVE here (everything else goes
// to Garage), and this carries it out on Garage as a copy and a delete, which is what the
// rename amounts to.
type wasabi struct {
	key, secret, endpoint, region string
}

func init() {
	w := &wasabi{
		key:      os.Getenv("WASABI_S3_KEY"),
		secret:   os.Getenv("WASABI_S3_SECRET"),
		endpoint: strings.TrimRight(os.Getenv("WASABI_S3_ENDPOINT"), "/"),
		region:   os.Getenv("WASABI_S3_REGION"),
	}
	register(w, "garage")
}

func (w *wasabi) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != "MOVE" {
		http.Error(rw, "only MOVE is served here", http.StatusMethodNotAllowed)
		return
	}
	if w.secret == "" || w.endpoint == "" {
		http.Error(rw, "WASABI_S3_* is not set", http.StatusInternalServerError)
		return
	}
	// Wasabi's signature is S3's version 2 over the verb, the date and the path.
	date := r.Header.Get("Date")
	mac := hmac.New(sha1.New, []byte(w.secret))
	fmt.Fprintf(mac, "MOVE\n\n\n%s\n%s", date, r.URL.EscapedPath())
	want := "AWS " + w.key + ":" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte(want)) {
		http.Error(rw, "SignatureDoesNotMatch", http.StatusForbidden)
		return
	}
	bucket, from, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	to := strings.TrimPrefix(r.Header.Get("Destination"), "/")
	if !ok || from == "" || to == "" {
		http.Error(rw, "MOVE needs /bucket/key and a Destination", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Overwrite") != "true" {
		if code, err := w.s3("HEAD", bucket, to, nil); err == nil && code == http.StatusOK {
			http.Error(rw, "destination exists", http.StatusPreconditionFailed)
			return
		}
	}
	copySource := map[string]string{"x-amz-copy-source": "/" + bucket + "/" + s3Escape(from)}
	if code, err := w.s3("PUT", bucket, to, copySource); err != nil || code != http.StatusOK {
		http.Error(rw, fmt.Sprintf("copy failed: %d %v", code, err), statusOr(code, http.StatusBadGateway))
		return
	}
	if code, err := w.s3("DELETE", bucket, from, nil); err != nil || code/100 != 2 {
		http.Error(rw, fmt.Sprintf("delete failed: %d %v", code, err), statusOr(code, http.StatusBadGateway))
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func statusOr(code, fallback int) int {
	if code >= 400 {
		return code
	}
	return fallback
}

// s3Escape escapes each path segment as S3 expects in a canonical URI and a copy source.
func s3Escape(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(p), "+", "%2B")
	}
	return strings.Join(parts, "/")
}

// s3 sends a path-style request to the S3 endpoint, signed with signature version 4.
func (w *wasabi) s3(method, bucket, key string, headers map[string]string) (int, error) {
	path := "/" + bucket + "/" + s3Escape(key)
	req, err := http.NewRequest(method, w.endpoint+path, nil)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	amzDate, day := now.Format("20060102T150405Z"), now.Format("20060102")
	payload := hex.EncodeToString(sha256Sum(nil))
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payload)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	names := []string{"host"}
	for k := range req.Header {
		names = append(names, strings.ToLower(k))
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		v := req.URL.Host
		if n != "host" {
			v = strings.TrimSpace(req.Header.Get(n))
		}
		canonHeaders.WriteString(n + ":" + v + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{method, path, "", canonHeaders.String(), signed, payload}, "\n")
	scope := day + "/" + w.region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sha256Sum([]byte(canonical)))
	k := hmacSHA256([]byte("AWS4"+w.secret), day)
	for _, part := range []string{w.region, "s3", "aws4_request"} {
		k = hmacSHA256(k, part)
	}
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		w.key, scope, signed, hex.EncodeToString(hmacSHA256(k, toSign))))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	// A copy can fail with 200 and an error document.
	if method == "PUT" && strings.Contains(string(body), "<Error>") {
		return http.StatusBadGateway, fmt.Errorf("%s", body)
	}
	return resp.StatusCode, nil
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
