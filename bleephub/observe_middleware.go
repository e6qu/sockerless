package bleephub

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
)

// observeMiddleware tees every response through the registered
// responseObserver. Hijacked exchanges (runner long-poll WebSockets)
// pass through unobserved — they carry no HTTP response body.
func (s *Server) observeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &observeWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !rec.hijacked {
			s.responseObserver(r, rec.status, rec.Header(), rec.body.Bytes())
		}
	})
}

type observeWriter struct {
	http.ResponseWriter
	status   int
	body     bytes.Buffer
	hijacked bool
}

func (w *observeWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *observeWriter) Write(b []byte) (int, error) {
	if !w.hijacked {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *observeWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *observeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	w.hijacked = true
	return h.Hijack()
}
