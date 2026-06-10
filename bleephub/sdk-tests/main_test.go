// Package sdktests exercises bleephub through the official google/go-github
// SDK. The point is wire fidelity: go-github decodes every response into typed
// structs, so a shape or field-name mismatch surfaces here as a decode error or
// a zero-valued field that an assertion catches. Setup-only resources that
// have no GitHub-real creation API on bleephub (orgs, apps) are provisioned via
// bleephub's /internal/* sim-control endpoints with a raw authenticated POST;
// everything GitHub-real goes through the typed SDK client.
package sdktests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	github "github.com/google/go-github/v88/github"
)

const adminToken = "bleephub-admin-token-00000000000000000000"

var (
	// baseURL is the http://host:port root of the running bleephub binary.
	baseURL string
	// client is the package-global authenticated go-github client pointed at
	// bleephub's GHES-style /api/v3/ surface.
	client *github.Client
	// rawHTTP is a plain client used for the /internal/* sim-control fixtures
	// (org/app creation) that go-github has no method for.
	rawHTTP = &http.Client{Timeout: 30 * time.Second}
)

// ctx is a convenience background context for SDK calls.
func ctx() context.Context { return context.Background() }

// uniqueCounter backs uniqueName so concurrent (t.Parallel) tests never collide
// on a repo/label/etc. name.
var uniqueCounter int64

// uniqueName returns a deterministic-prefix, globally-unique identifier safe to
// use as a repo or other resource name.
func uniqueName(prefix string) string {
	n := atomic.AddInt64(&uniqueCounter, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano()%1_000_000, n)
}

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain setup failed:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	// Build the bleephub binary from the parent module. Mirrors the AWS
	// sim-tests TestMain: build once, run the real binary, talk to it over HTTP.
	bin := "./bleephub-server"
	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}
	build := exec.Command("go", "build", "-tags", "noui", "-o", bin, "./cmd")
	build.Dir = ".." // the bleephub module root
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return 1, fmt.Errorf("build bleephub: %v\n%s", err, out)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 1, fmt.Errorf("find free port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	cmd := exec.Command(bin, "--addr", fmt.Sprintf(":%d", port))
	cmd.Env = append(os.Environ(), "BLEEPHUB_ADMIN_TOKEN="+adminToken)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("start bleephub: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Poll /health until ready.
	ready := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		return 1, fmt.Errorf("bleephub did not become ready at %s/health", baseURL)
	}

	// Build the authenticated go-github client. bleephub is GHES-style: REST at
	// /api/v3/, uploads reuse the base. WithEnterpriseURLs appends api/v3/ to a
	// trailing-slash base, and api/uploads/ to the upload base.
	client, err = github.NewClient(
		github.WithAuthToken(adminToken),
		github.WithEnterpriseURLs(baseURL+"/", baseURL+"/"),
	)
	if err != nil {
		return 1, fmt.Errorf("new go-github client: %w", err)
	}

	return m.Run(), nil
}

// internalPost issues an authenticated POST to a bleephub /internal/* endpoint
// and decodes the JSON response into out (if non-nil). It is used only for
// fixture setup of resources go-github cannot create (orgs, apps).
func internalPost(t *testing.T, path string, body, out interface{}) int {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal internal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, rdr)
	if err != nil {
		t.Fatalf("new internal request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := rawHTTP.Do(req)
	if err != nil {
		t.Fatalf("internal POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode internal response %s (%s): %v", path, raw, err)
		}
	}
	return resp.StatusCode
}

// createRepo creates a repo owned by the authenticated user (org="") and
// returns it. It fails the test on error so callers can assume success.
func createRepo(t *testing.T, name string) *github.Repository {
	t.Helper()
	repo, _, err := client.Repositories.Create(ctx(), "", &github.Repository{
		Name:        github.Ptr(name),
		Description: github.Ptr("created by sdk-tests"),
		AutoInit:    github.Ptr(false),
	})
	if err != nil {
		t.Fatalf("Repositories.Create(%q): %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = client.Repositories.Delete(ctx(), "admin", name)
	})
	return repo
}
