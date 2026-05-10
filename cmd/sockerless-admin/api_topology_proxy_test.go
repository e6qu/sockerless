package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// upstreamServerOnLocalhost spins up a real http.Server bound to a
// loopback port chosen by the OS. The proxy handler always dials
// http://localhost:<inst.Port>/, so the test must register an
// instance whose Port matches the listener.
func upstreamServerOnLocalhost(t *testing.T, handler http.HandlerFunc) (port int, stop func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	u, err := url.Parse(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("parse upstream URL: %v", err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		srv.Close()
		t.Fatalf("parse upstream port: %v", err)
	}
	return p, srv.Close
}

func setupProxyServer(t *testing.T, port int) *http.ServeMux {
	t.Helper()
	tmp := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"), "")
	if err := mgr.LoadOrMigrate(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{{
		Name: "p",
		Instances: []Instance{
			{Name: "sim-aws", Kind: InstanceKindSim, Cloud: CloudAWS, Port: port},
		},
	}}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, nil)
	return mux
}

func TestProxyForwardsGET(t *testing.T) {
	port, stop := upstreamServerOnLocalhost(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			t.Errorf("upstream path = %q, want /v1/health", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("upstream method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	defer stop()

	mux := setupProxyServer(t, port)
	body, _ := json.Marshal(proxyRequest{Method: "GET", Path: "/v1/health"})
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/sim-aws/proxy",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got proxyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != http.StatusOK {
		t.Errorf("upstream status = %d, want 200", got.Status)
	}
	if got.Body != `{"status":"ok"}` {
		t.Errorf("body = %q, want {\"status\":\"ok\"}", got.Body)
	}
	if got.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header missing: %+v", got.Headers)
	}
	if got.DurationMs < 0 {
		t.Errorf("duration negative: %d", got.DurationMs)
	}
}

func TestProxyForwardsPOSTWithBodyAndHeaders(t *testing.T) {
	port, stop := upstreamServerOnLocalhost(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("X-Custom") != "console" {
			t.Errorf("X-Custom header missing")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"hello":"world"}` {
			t.Errorf("body = %q", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`accepted`))
	})
	defer stop()

	mux := setupProxyServer(t, port)
	body, _ := json.Marshal(proxyRequest{
		Method:  "POST",
		Path:    "/v1/echo",
		Headers: map[string]string{"X-Custom": "console"},
		Body:    `{"hello":"world"}`,
	})
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/sim-aws/proxy",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var got proxyResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Status != http.StatusCreated {
		t.Errorf("upstream status = %d, want 201", got.Status)
	}
	if got.Body != "accepted" {
		t.Errorf("body = %q", got.Body)
	}
}

func TestProxyInstanceNotFound(t *testing.T) {
	mux := setupProxyServer(t, 1)
	body, _ := json.Marshal(proxyRequest{Method: "GET", Path: "/x"})
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/missing/proxy",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestProxyRejectsBadPath(t *testing.T) {
	mux := setupProxyServer(t, 1)
	cases := []proxyRequest{
		{Method: "GET", Path: ""},
		{Method: "GET", Path: "no-leading-slash"},
	}
	for _, tc := range cases {
		body, _ := json.Marshal(tc)
		req := httptest.NewRequest("POST",
			"/api/v1/topology/projects/p/instances/sim-aws/proxy",
			bytes.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("path=%q status=%d, want 400", tc.Path, w.Code)
		}
	}
}

func TestProxyUpstreamUnreachable(t *testing.T) {
	// Pick a port that nothing is listening on.
	mux := setupProxyServer(t, 1) // privileged port; connect refused
	body, _ := json.Marshal(proxyRequest{Method: "GET", Path: "/x"})
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/sim-aws/proxy",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "connect") && !strings.Contains(resp["error"], "refused") &&
		!strings.Contains(resp["error"], "dial") {
		// Different OSes word the dial error differently — just assert
		// that *some* error was surfaced.
		if resp["error"] == "" {
			t.Errorf("expected error message, got empty")
		}
	}
}

// Sanity: keep test runtime bounded.
func TestProxyDurationCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skip slow timing test in -short mode")
	}
	port, stop := upstreamServerOnLocalhost(t, func(w http.ResponseWriter, r *http.Request) {
		// Respond immediately.
		w.WriteHeader(http.StatusOK)
	})
	defer stop()

	mux := setupProxyServer(t, port)
	body, _ := json.Marshal(proxyRequest{Method: "GET", Path: "/"})
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/sim-aws/proxy",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	start := time.Now()
	mux.ServeHTTP(w, req)
	if elapsed := time.Since(start); elapsed > proxyTimeout {
		t.Errorf("call took %v, expected well under %v", elapsed, proxyTimeout)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}
