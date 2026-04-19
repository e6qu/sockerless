// http-trace is a throwaway diagnostic tool that logs every HTTP
// request and response flowing through it. Used by the Phase 86
// bug-fix sprint (A.1) to diagnose BUG-698: docker CLI hangs between
// POST /containers/create and POST /start against the sockerless ECS
// backend. Running docker CLI against this proxy captures the exact
// wire sequence for inspection.
//
// Usage:
//
//	go run ./tools/http-trace -listen :12375 -upstream http://127.0.0.1:2375
//	DOCKER_HOST=tcp://127.0.0.1:12375 docker run -d alpine echo hi
//
// The proxy supports HTTP/1.1 hijacked connections (attach, exec) by
// switching to raw TCP bridging on 101 Switching Protocols.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var reqID atomic.Int64

func main() {
	listen := flag.String("listen", ":12375", "listen address")
	upstream := flag.String("upstream", "http://127.0.0.1:2375", "upstream URL")
	flag.Parse()

	u, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("bad upstream: %v", err)
	}
	log.SetOutput(os.Stderr)
	log.Printf("http-trace: listening on %s -> %s", *listen, u)

	srv := &http.Server{
		Addr:    *listen,
		Handler: newTracer(u),
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

type tracer struct{ upstream *url.URL }

func newTracer(u *url.URL) *tracer { return &tracer{upstream: u} }

func (t *tracer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := reqID.Add(1)
	start := time.Now()

	// Read + buffer the request body so we can log it and still forward.
	var reqBody []byte
	if r.Body != nil {
		var err error
		reqBody, err = io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[%d] ERR read req body: %v", id, err)
		}
		_ = r.Body.Close()
	}

	// Log the request.
	log.Printf("[%d] >>> %s %s", id, r.Method, r.RequestURI)
	logHeaders(id, ">", r.Header)
	logBody(id, ">", reqBody)

	// Decide whether this is a hijacking upgrade request.
	isUpgrade := strings.EqualFold(r.Header.Get("Connection"), "Upgrade") ||
		strings.EqualFold(r.Header.Get("Upgrade"), "tcp")

	// Build upstream request.
	upURL := *t.upstream
	upURL.Path = r.URL.Path
	upURL.RawQuery = r.URL.RawQuery
	req, err := http.NewRequest(r.Method, upURL.String(), bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if isUpgrade {
		t.proxyHijacked(id, w, req, reqBody)
		return
	}

	// Use a fresh transport (no connection pooling) so each request
	// gets a clean upstream conn — clearer traces.
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Printf("[%d] ERR roundtrip: %v", id, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Tee response body so we can log + forward.
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[%d] <<< %s (%v)", id, resp.Status, time.Since(start))
	logHeaders(id, "<", resp.Header)
	logBody(id, "<", respBody)

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// proxyHijacked bridges a hijackable request (Connection: Upgrade) by
// opening a raw TCP connection to the upstream, writing the request
// verbatim, reading the response headers, and then byte-copying both
// directions until either end closes. All of the traffic is logged.
func (t *tracer) proxyHijacked(id int64, w http.ResponseWriter, req *http.Request, reqBody []byte) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[%d] ERR: ResponseWriter not a Hijacker", id)
		http.Error(w, "hijack not supported", 500)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		log.Printf("[%d] ERR: hijack client: %v", id, err)
		return
	}
	defer clientConn.Close()

	host := t.upstream.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}
	upConn, err := net.Dial("tcp", host)
	if err != nil {
		log.Printf("[%d] ERR: dial upstream: %v", id, err)
		return
	}
	defer upConn.Close()

	// Write the original request line + headers to upstream.
	if err := req.Write(upConn); err != nil {
		log.Printf("[%d] ERR: write req to upstream: %v", id, err)
		return
	}
	log.Printf("[%d] hijack: wrote request to upstream (%s %s)", id, req.Method, req.URL.Path)

	// Read upstream's initial HTTP response (the 101 + any response line).
	upReader := bufio.NewReader(upConn)
	statusLine, err := upReader.ReadString('\n')
	if err != nil {
		log.Printf("[%d] ERR: read status line: %v", id, err)
		return
	}
	log.Printf("[%d] hijack: upstream status line: %q", id, strings.TrimSpace(statusLine))
	// Forward status line.
	if _, err := clientBuf.WriteString(statusLine); err != nil {
		log.Printf("[%d] ERR: write status to client: %v", id, err)
		return
	}
	// Read + forward response headers verbatim.
	for {
		line, err := upReader.ReadString('\n')
		if err != nil {
			log.Printf("[%d] ERR: read header: %v", id, err)
			return
		}
		_, _ = clientBuf.WriteString(line)
		if strings.TrimSpace(line) == "" {
			break
		}
		log.Printf("[%d] hijack: upstream header: %s", id, strings.TrimSpace(line))
	}
	_ = clientBuf.Flush()
	log.Printf("[%d] hijack: headers flushed to client; starting bidirectional copy", id)

	// Bidirectional raw-byte copy with logging.
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(newLogger(id, "hijack C->U"), io.TeeReader(clientConn, newLogger(id, "hijack C->U raw")))
		_ = n
		done <- struct{}{}
	}()
	go func() {
		// Drain any remaining buffered upstream bytes first.
		if upReader.Buffered() > 0 {
			peeked, _ := upReader.Peek(upReader.Buffered())
			log.Printf("[%d] hijack U->C buffered %d bytes: %q", id, len(peeked), truncate(peeked, 200))
			_, _ = clientConn.Write(peeked)
			_, _ = upReader.Discard(len(peeked))
		}
		n, _ := io.Copy(clientConn, io.TeeReader(upConn, newLogger(id, "hijack U->C raw")))
		_ = n
		done <- struct{}{}
	}()
	<-done
	log.Printf("[%d] hijack: one direction closed, ending bridge", id)
}

func logHeaders(id int64, dir string, h http.Header) {
	for k, vs := range h {
		for _, v := range vs {
			log.Printf("[%d] %s header %s: %s", id, dir, k, v)
		}
	}
}

func logBody(id int64, dir string, body []byte) {
	if len(body) == 0 {
		log.Printf("[%d] %s body (empty)", id, dir)
		return
	}
	log.Printf("[%d] %s body (%d bytes): %s", id, dir, len(body), truncate(body, 1000))
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + fmt.Sprintf("… (+%d more)", len(b)-max)
}

// newLogger returns an io.Writer that logs every write with a label.
func newLogger(id int64, label string) io.Writer {
	return &byteLogger{id: id, label: label}
}

type byteLogger struct {
	id    int64
	label string
}

func (b *byteLogger) Write(p []byte) (int, error) {
	// Dump as hex if non-printable.
	s := string(p)
	if !isPrintable(p) {
		s = fmt.Sprintf("%x", p)
	}
	log.Printf("[%d] %s %d bytes: %s", b.id, b.label, len(p), truncate([]byte(s), 400))
	return len(p), nil
}

func isPrintable(p []byte) bool {
	for _, b := range p {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			return false
		}
	}
	return true
}

var _ = httputil.DumpRequest // keep import for ad-hoc tweaks
