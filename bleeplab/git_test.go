package bleeplab

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/rs/zerolog"
)

// TestGitCloneSeededProject proves a real git client can clone a project repo
// served over bleeplab's smart-HTTP, exactly as a gitlab-runner does to
// materialize CI_PROJECT_DIR. The clone is the gate the GitLab ECS cell needs.
func TestGitCloneSeededProject(t *testing.T) {
	s := NewServer(":0", zerolog.Nop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	// Create a project and commit a .gitlab-ci.yml via the control-plane API.
	resp, body := do(t, ts, http.MethodPost, "/api/v4/projects", map[string]any{"name": "demo"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: %d %s", resp.StatusCode, body)
	}
	var proj struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &proj); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	ci := "stages: [build]\nbuild-job:\n  stage: build\n  script:\n    - echo hi\n"
	resp, body = do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/repository/commits",
		map[string]any{"branch": "main", "actions": []map[string]string{{"file_path": ".gitlab-ci.yml", "content": ci}}}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("commit: %d %s", resp.StatusCode, body)
	}
	var commit struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if commit.ID == "" {
		t.Fatal("commit returned empty SHA")
	}

	// Clone over HTTP with a real git client (the gitlab-ci-token the runner
	// would send is accepted, matching GitLab).
	repo, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:  ts.URL + "/root/demo.git",
		Auth: &githttp.BasicAuth{Username: "gitlab-ci-token", Password: "tok"},
	})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.Hash().String() != commit.ID {
		t.Fatalf("clone HEAD %s != commit SHA %s", head.Hash(), commit.ID)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	f, err := wt.Filesystem.Open(".gitlab-ci.yml")
	if err != nil {
		t.Fatalf("open cloned file: %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(got) != ci {
		t.Fatalf("cloned .gitlab-ci.yml mismatch:\n got %q\nwant %q", got, ci)
	}

	// A second commit is additive (GitLab create/update semantics): the
	// earlier file must survive. Clone again and assert both files exist.
	resp, body = do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/repository/commits",
		map[string]any{"branch": "main", "actions": []map[string]string{{"file_path": "README.md", "content": "hello"}}}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second commit: %d %s", resp.StatusCode, body)
	}
	repo2, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:  ts.URL + "/root/demo.git",
		Auth: &githttp.BasicAuth{Username: "gitlab-ci-token", Password: "tok"},
	})
	if err != nil {
		t.Fatalf("re-clone: %v", err)
	}
	wt2, err := repo2.Worktree()
	if err != nil {
		t.Fatalf("worktree2: %v", err)
	}
	for _, name := range []string{".gitlab-ci.yml", "README.md"} {
		if _, err := wt2.Filesystem.Stat(name); err != nil {
			t.Fatalf("expected %s in repo after second commit: %v", name, err)
		}
	}
}
