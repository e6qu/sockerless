package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// wrappedTestHandler builds the server's real request pipeline — the exact
// middleware chain ListenAndServe runs (prefix-strip → internal-auth →
// gh-headers), minus only the optional otel/response-observer wrappers — so
// the concurrency storms exercise the actual production handler + store code,
// not a stripped-down subset.
var (
	wrappedOnce sync.Once
	wrappedH    http.Handler
)

func wrappedTestHandler(s *Server) http.Handler {
	wrappedOnce.Do(func() {
		wrappedH = s.ghHeadersMiddleware(s.prefixStripMiddleware(s.internalAuthMiddleware(s.mux)))
	})
	return wrappedH
}

// drainClose fully reads then closes a response body — ordinary client
// resource hygiene (an unread body leaks the reader). Applies to both the
// in-process recorder responses and the TCP-seeded gh* helper responses.
func drainClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// doReq issues one authenticated GitHub-API request through the server's real
// wrapped handler, in process. It deliberately does NOT go over a TCP socket:
// the goal is to prove the app's handlers and store withstand heavy concurrent
// load, so the request is dispatched straight into the production pipeline with
// no client of any kind between the storm and the code under test. It never
// touches *testing.T, so many goroutines may call it at once, and the shared
// store it drives is the real object the -race detector instruments.
// The caller closes resp.Body (use drainClose).
func doReq(h http.Handler, method, path, token string, body interface{}) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result(), nil
}
