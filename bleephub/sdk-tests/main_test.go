// Package sdktests exercises Bleephub through the official google/go-github
// software development kit. The point is wire fidelity: go-github decodes every
// response into typed structs, so a shape or field-name mismatch surfaces here
// as a decode error or a zero-valued field that an assertion catches.
// Setup-only resources that have no GitHub-real typed creation method are
// provisioned through the closest real GitHub flow when one exists, and through
// Bleephub's operator-only /internal/* endpoints only when GitHub exposes no
// equivalent typed method for the resource.
package sdktests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	github "github.com/google/go-github/v88/github"
)

const adminToken = "bleephub-admin-token-00000000000000000000"

var (
	// baseURL is the http://host:port root of the running Bleephub binary.
	baseURL string
	// client is the package-global authenticated go-github client pointed at
	// Bleephub's GitHub Enterprise Server-style /api/v3/ surface.
	client *github.Client
	// rawHTTP is a plain client used for operator-only fixture setup that
	// go-github has no method for.
	rawHTTP = &http.Client{Timeout: 30 * time.Second}
	// rawNoRedirectHTTP reads browser-flow redirects instead of following them.
	rawNoRedirectHTTP = &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

// ctx is a convenience background context for software development kit calls.
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
	// Build the Bleephub binary from the parent module. Mirrors the Amazon Web
	// Services local cloud implementation tests TestMain: build once, run the
	// real binary, talk to it over Hypertext Transfer Protocol.
	bin := "./bleephub-server"
	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}
	build := exec.Command("go", "build", "-tags", "noui", "-o", bin, "./cmd")
	build.Dir = ".." // the Bleephub module root
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return 1, fmt.Errorf("build Bleephub: %v\n%s", err, out)
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
		return 1, fmt.Errorf("start Bleephub: %w", err)
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
		return 1, fmt.Errorf("Bleephub did not become ready at %s/health", baseURL)
	}

	// Build the authenticated go-github client. Bleephub is GitHub Enterprise
	// Server-style: Representational State Transfer at /api/v3/, uploads reuse
	// the base. WithEnterpriseURLs appends api/v3/ to a trailing-slash base, and
	// api/uploads/ to the upload base.
	client, err = github.NewClient(
		github.WithAuthToken(adminToken),
		github.WithEnterpriseURLs(baseURL+"/", baseURL+"/"),
	)
	if err != nil {
		return 1, fmt.Errorf("new go-github client: %w", err)
	}

	return m.Run(), nil
}

// internalPost issues an authenticated POST to a Bleephub /internal/* endpoint
// and decodes the JavaScript Object Notation response into out when out is
// non-nil. It is used only for operator-only fixture setup of resources
// go-github cannot create.
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

func createGitHubAppViaManifest(t *testing.T, name string, permissions map[string]string, out interface{}) int {
	t.Helper()
	manifest := map[string]interface{}{
		"name":                name,
		"url":                 "https://example.test/app",
		"redirect_url":        "https://example.test/callback",
		"default_permissions": permissions,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal GitHub App manifest: %v", err)
	}
	form := url.Values{"manifest": {string(manifestJSON)}}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/settings/apps/new", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new GitHub App manifest request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := rawNoRedirectHTTP.Do(req)
	if err != nil {
		t.Fatalf("post GitHub App manifest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		return resp.StatusCode
	}
	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse GitHub App manifest redirect: %v", err)
	}
	code := location.Query().Get("code")
	if code == "" {
		t.Fatal("GitHub App manifest redirect did not include a conversion code")
	}
	req, err = http.NewRequest(http.MethodPost, baseURL+"/api/v3/app-manifests/"+code+"/conversions", nil)
	if err != nil {
		t.Fatalf("new GitHub App manifest conversion request: %v", err)
	}
	resp, err = rawHTTP.Do(req)
	if err != nil {
		t.Fatalf("convert GitHub App manifest: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode GitHub App manifest conversion response (%s): %v", raw, err)
		}
	}
	return resp.StatusCode
}

func installGitHubAppViaBrowser(t *testing.T, appSlug, targetLogin, selection string, repoIDs []int64, out interface{}) int {
	t.Helper()
	form := url.Values{}
	if targetLogin != "" {
		form.Set("target_login", targetLogin)
	}
	if selection != "" {
		form.Set("repository_selection", selection)
	}
	for _, id := range repoIDs {
		form.Add("repository_ids", fmt.Sprint(id))
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/apps/"+appSlug+"/installations/new", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new GitHub App installation request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := rawHTTP.Do(req)
	if err != nil {
		t.Fatalf("install GitHub App: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode GitHub App installation response (%s): %v", raw, err)
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
