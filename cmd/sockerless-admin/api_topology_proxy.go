package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// proxyRequest is the JSON body the admin UI's API console panel
// posts. Path is appended to http://localhost:<inst.Port>; method,
// headers, and body are forwarded as-is.
type proxyRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// proxyResponse mirrors the upstream response in a JSON-friendly
// shape: response body is returned as a string regardless of
// content-type so the UI can render it without binary fuss.
type proxyResponse struct {
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMs int64             `json:"duration_ms"`
}

const (
	// proxyMaxBody caps the upstream response body the admin will read
	// back to the UI — large blobs would balloon admin memory and the
	// UI cannot render them usefully.
	proxyMaxBody = 4 * 1024 * 1024
	// proxyTimeout caps a single proxied request. The admin's console
	// is for poking at running components, not for kicking off long
	// jobs — keep things bounded.
	proxyTimeout = 30 * time.Second
)

// handleInstanceProxy forwards an arbitrary HTTP request to the
// instance's local port. Used by the admin UI's "API console" panel
// to inspect / poke at any running instance without dealing with
// browser CORS — the request is server-side from admin to the
// instance.
//
// Components stay decoupled: this targets the instance's existing
// public surface (whatever the component already exposes on its port)
// — components do not grow new endpoints to support the console.
func handleInstanceProxy(mgr *TopologyManager, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instance := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instance)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instance + " not found",
			})
			return
		}
		if ref.Instance.Port <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "instance has no port — cannot proxy",
			})
			return
		}

		var req proxyRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body: " + err.Error(),
			})
			return
		}
		if req.Method == "" {
			req.Method = "GET"
		}
		if req.Path == "" || !strings.HasPrefix(req.Path, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "path is required and must start with /",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), proxyTimeout)
		defer cancel()

		url := fmt.Sprintf("http://localhost:%d%s", ref.Instance.Port, req.Path)
		var body io.Reader
		if req.Body != "" && req.Method != "GET" && req.Method != "HEAD" {
			body = bytes.NewReader([]byte(req.Body))
		}
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "request build failed: " + err.Error(),
			})
			return
		}
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		start := time.Now()
		resp, err := client.Do(httpReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, proxyMaxBody))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "response read failed: " + err.Error(),
			})
			return
		}
		respHeaders := make(map[string]string, len(resp.Header))
		for k, v := range resp.Header {
			respHeaders[k] = strings.Join(v, ", ")
		}
		writeJSON(w, http.StatusOK, proxyResponse{
			Status:     resp.StatusCode,
			StatusText: resp.Status,
			Headers:    respHeaders,
			Body:       string(respBody),
			DurationMs: time.Since(start).Milliseconds(),
		})
	}
}
