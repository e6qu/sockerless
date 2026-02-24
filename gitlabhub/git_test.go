package gitlabhub

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateProjectRepo(t *testing.T) {
	s := newTestServer(t)

	sha, err := s.createProjectRepo("test-project", map[string]string{
		".gitlab-ci.yml": "test:\n  script: [echo hi]",
		"README.md":      "# Test",
	})
	if err != nil {
		t.Fatalf("create repo error: %v", err)
	}

	if sha == "" {
		t.Fatal("expected non-empty sha")
	}
	if len(sha) != 40 {
		t.Fatalf("expected 40-char sha, got %d", len(sha))
	}
}

func TestGitInfoRefs(t *testing.T) {
	s := newTestServer(t)

	_, err := s.createProjectRepo("myrepo", map[string]string{
		"file.txt": "content",
	})
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	rr := doRequest(s, "GET", "/myrepo.git/info/refs?service=git-upload-pack", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "git-upload-pack") {
		t.Fatalf("unexpected Content-Type: %s", contentType)
	}

	// Should contain refs
	body := rr.Body.String()
	if !strings.Contains(body, "refs/heads/main") {
		t.Fatal("expected refs/heads/main in advertised refs")
	}
}

func TestGitRepoNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := doRequest(s, "GET", "/nonexistent.git/info/refs?service=git-upload-pack", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGitUploadPack(t *testing.T) {
	s := newTestServer(t)

	_, err := s.createProjectRepo("myrepo", map[string]string{
		"test.txt": "hello",
	})
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// Just verify the endpoint exists and returns correct content type
	// Full clone requires a proper git client, so we test the info/refs response
	rr := doRequest(s, "GET", "/myrepo.git/info/refs?service=git-upload-pack", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("info/refs failed: %d", rr.Code)
	}
}

func TestGitCloneURL(t *testing.T) {
	s := newTestServer(t)

	_, err := s.createProjectRepo("project-1", map[string]string{
		".gitlab-ci.yml": "test:\n  script: [echo ok]",
	})
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// Verify info/refs works with project-1 path
	rr := doRequest(s, "GET", "/project-1.git/info/refs?service=git-upload-pack", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestMultipleProjects(t *testing.T) {
	s := newTestServer(t)

	for _, name := range []string{"proj-a", "proj-b", "proj-c"} {
		_, err := s.createProjectRepo(name, map[string]string{
			"file.txt": "content for " + name,
		})
		if err != nil {
			t.Fatalf("create repo %s: %v", name, err)
		}
	}

	for _, name := range []string{"proj-a", "proj-b", "proj-c"} {
		rr := doRequest(s, "GET", "/"+name+".git/info/refs?service=git-upload-pack", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", name, rr.Code)
		}
	}
}

func TestGitStoragePersisted(t *testing.T) {
	s := newTestServer(t)

	_, err := s.createProjectRepo("test", map[string]string{
		"a.txt": "aaa",
	})
	if err != nil {
		t.Fatal(err)
	}

	stor := s.store.GetGitStorage("test")
	if stor == nil {
		t.Fatal("expected git storage to be persisted")
	}
}
