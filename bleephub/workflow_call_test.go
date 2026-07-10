package bleephub

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitFilesToStorage commits a set of files in ONE commit at HEAD of
// the repo's git storage, creating owner + repo when missing.
func commitFilesToStorage(t *testing.T, s *Server, repoFullName string, files map[string]string) {
	t.Helper()
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		t.Fatalf("expected owner/repo, got %q", repoFullName)
	}
	if s.store.UsersByLogin[parts[0]] == nil {
		s.store.mu.Lock()
		user := &User{ID: s.store.NextUser, Login: parts[0], Type: "User", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		s.store.NextUser++
		s.store.Users[user.ID] = user
		s.store.UsersByLogin[user.Login] = user
		s.store.mu.Unlock()
	}
	if s.store.GetRepo(parts[0], parts[1]) == nil {
		s.store.CreateRepo(s.store.UsersByLogin[parts[0]], parts[1], "", false)
	}
	storer := s.store.GetGitStorage(parts[0], parts[1])
	if storer == nil {
		t.Fatalf("no git storage for %s", repoFullName)
	}
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		repo, err = git.Open(storer, fs)
		if err != nil {
			t.Fatalf("init/open repo: %v", err)
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	for path, body := range files {
		if idx := strings.LastIndex(path, "/"); idx > 0 {
			if err := fs.MkdirAll(path[:idx], 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", path[:idx], err)
			}
		}
		f, err := fs.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		_, _ = f.Write([]byte(body))
		_ = f.Close()
		if _, err := wt.Add(path); err != nil {
			t.Fatalf("git add %s: %v", path, err)
		}
	}
	commitHash, err := wt.Commit("test files", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	mainRef := plumbing.NewBranchReferenceName("main")
	if err := storer.SetReference(plumbing.NewHashReference(mainRef, commitHash)); err != nil {
		t.Fatalf("set main ref: %v", err)
	}
	if err := storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
}

const calledWorkflowYAML = `name: called
on:
  workflow_call:
    inputs:
      env:
        type: string
        required: true
      replicas:
        type: number
        default: 2
    secrets:
      deploy-key:
        required: false
    outputs:
      url:
        value: ${{ jobs.publish.outputs.url }}
jobs:
  publish:
    runs-on: ubuntu-latest
    outputs:
      url: ${{ steps.out.outputs.url }}
    steps:
      - run: echo publish
`

const callerWorkflowYAML = `name: caller
on: [push]
jobs:
  setup:
    runs-on: ubuntu-latest
    steps:
      - run: echo setup
  deploy:
    needs: [setup]
    uses: ./.github/workflows/called.yml
    with:
      env: prod-${{ needs.setup.outputs.version }}
  notify:
    needs: [deploy]
    runs-on: ubuntu-latest
    steps:
      - run: echo done
`

func TestWorkflowCallEndToEnd(t *testing.T) {
	repoKey := "callowner/call-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitFilesToStorage(t, testServer, repoKey, map[string]string{
		".github/workflows/caller.yml": callerWorkflowYAML,
		".github/workflows/called.yml": calledWorkflowYAML,
	})

	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	testServer.store.mu.RLock()
	var wf *Workflow
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey && w.Name == "caller" {
			wf = w
		}
	}
	testServer.store.mu.RUnlock()
	if wf == nil {
		t.Fatal("caller workflow not triggered")
	}

	// Expanded graph: setup, notify, deploy/__call (gate), deploy/publish, deploy (collector).
	for _, key := range []string{"setup", "notify", "deploy/__call", "deploy/publish", "deploy"} {
		if wf.Jobs[key] == nil {
			t.Fatalf("missing job %q; have %v", key, jobKeys(wf))
		}
	}
	if !wf.Jobs["deploy/__call"].Hidden || !wf.Jobs["deploy"].Hidden {
		t.Error("gate and collector must be hidden")
	}
	if wf.Jobs["deploy/publish"].Hidden {
		t.Error("called job must not be hidden")
	}
	if got := wf.Jobs["deploy/publish"].DisplayName; got != "deploy / publish" {
		t.Errorf("called job display name = %q, want 'deploy / publish'", got)
	}

	// Complete setup with an output; the gate must resolve `with:`.
	testServer.store.mu.Lock()
	wf.Jobs["setup"].Outputs["version"] = "1.2.3"
	setupID := wf.Jobs["setup"].JobID
	testServer.store.mu.Unlock()
	testServer.onJobCompleted(context.Background(), setupID, "Succeeded")

	testServer.store.mu.RLock()
	gate := wf.Jobs["deploy/__call"]
	publish := wf.Jobs["deploy/publish"]
	gateStatus := gate.Status
	publishStatus := publish.Status
	binding := publish.Def.Call
	testServer.store.mu.RUnlock()

	if gateStatus != JobStatusCompleted {
		t.Fatalf("gate status = %q, want completed", gateStatus)
	}
	if publishStatus != JobStatusQueued {
		t.Fatalf("publish status = %q, want queued", publishStatus)
	}
	resolved := binding.ResolvedInputs()
	if resolved["env"] != "prod-1.2.3" {
		t.Errorf("resolved input env = %v, want prod-1.2.3", resolved["env"])
	}
	if resolved["replicas"] != float64(2) {
		t.Errorf("resolved input replicas = %v (%T), want default 2", resolved["replicas"], resolved["replicas"])
	}

	// The called job's runner message carries the call inputs and the
	// caller-view needs context (no gate, unprefixed keys).
	msg, err := testServer.buildJobMessageFromDef("http://localhost", wf, publish, "p", "t", 1, "alpine:latest")
	if err != nil {
		t.Fatal(err)
	}
	ctxData := msg["contextData"].(map[string]interface{})
	if ctxData["inputs"] == nil {
		t.Error("called job message missing inputs context")
	}

	// Complete publish with the url output; the collector must map it.
	testServer.store.mu.Lock()
	publish.Outputs["url"] = "https://prod.example"
	publishID := publish.JobID
	testServer.store.mu.Unlock()
	testServer.onJobCompleted(context.Background(), publishID, "Succeeded")

	testServer.store.mu.RLock()
	collector := wf.Jobs["deploy"]
	notify := wf.Jobs["notify"]
	collectorStatus := collector.Status
	collectorURL := collector.Outputs["url"]
	notifyStatus := notify.Status
	testServer.store.mu.RUnlock()

	if collectorStatus != JobStatusCompleted {
		t.Fatalf("collector status = %q, want completed", collectorStatus)
	}
	if collectorURL != "https://prod.example" {
		t.Errorf("collector output url = %q, want mapped from called job", collectorURL)
	}
	if notifyStatus != JobStatusQueued {
		t.Fatalf("notify status = %q, want queued (needs.deploy satisfied)", notifyStatus)
	}

	// notify's needs context exposes the caller key with the mapped outputs.
	nmsg, err := testServer.buildJobMessageFromDef("http://localhost", wf, notify, "p2", "t2", 2, "alpine:latest")
	if err != nil {
		t.Fatal(err)
	}
	nctx := nmsg["contextData"].(map[string]interface{})
	needsJSON := nctx["needs"]
	if needsJSON == nil {
		t.Fatal("notify message missing needs context")
	}
}

func jobKeys(wf *Workflow) []string {
	keys := make([]string, 0, len(wf.Jobs))
	for k := range wf.Jobs {
		keys = append(keys, k)
	}
	return keys
}

func TestWorkflowCallSkipCascade(t *testing.T) {
	repoKey := "callskip/skip-repo"
	caller := `name: skip-caller
on: [push]
jobs:
  deploy:
    if: github.ref == 'refs/heads/release'
    uses: ./.github/workflows/called.yml
    with:
      env: prod
  notify:
    needs: [deploy]
    runs-on: ubuntu-latest
    steps:
      - run: echo done
`
	commitFilesToStorage(t, testServer, repoKey, map[string]string{
		".github/workflows/caller.yml": caller,
		".github/workflows/called.yml": calledWorkflowYAML,
	})

	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	testServer.store.mu.RLock()
	var wf *Workflow
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey {
			wf = w
		}
	}
	testServer.store.mu.RUnlock()
	if wf == nil {
		t.Fatal("workflow not triggered")
	}

	testServer.store.mu.RLock()
	defer testServer.store.mu.RUnlock()
	for _, key := range []string{"deploy/__call", "deploy/publish", "deploy", "notify"} {
		j := wf.Jobs[key]
		if j == nil {
			t.Fatalf("missing job %q", key)
		}
		if j.Status != JobStatusSkipped {
			t.Errorf("job %q status = %q, want skipped (gate if: false must cascade)", key, j.Status)
		}
	}
	if wf.Status != WorkflowStatusCompleted {
		t.Errorf("workflow status = %q, want completed", wf.Status)
	}
}

func TestWorkflowCallValidation(t *testing.T) {
	repoKey := "callval/val-repo"
	commitFilesToStorage(t, testServer, repoKey, map[string]string{
		".github/workflows/called.yml":     calledWorkflowYAML,
		".github/workflows/not-called.yml": "name: x\non: [push]\njobs:\n  a:\n    steps:\n      - run: echo a\n",
	})

	cases := []struct {
		name string
		job  *JobDef
		want string
	}{
		{"missing required input", &JobDef{Uses: "./.github/workflows/called.yml"}, "requires input"},
		{"unknown input", &JobDef{Uses: "./.github/workflows/called.yml",
			With: map[string]string{"env": "x", "bogus": "y"}}, "does not define input"},
		{"not workflow_call", &JobDef{Uses: "./.github/workflows/not-called.yml",
			With: map[string]string{}}, "does not declare on: workflow_call"},
		{"missing file", &JobDef{Uses: "./.github/workflows/nope.yml"}, "not found"},
	}
	for _, tc := range cases {
		def := &WorkflowDef{Name: "v", Jobs: map[string]*JobDef{"call": tc.job}}
		meta := &WorkflowEventMeta{EventName: "push", Repo: repoKey}
		_, err := testServer.submitWorkflow(context.Background(), "http://localhost", def, "alpine:latest", meta)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestWorkflowCallNestingDepthLimit(t *testing.T) {
	repoKey := "calldeep/deep-repo"
	// l1 → l2 → l3 → l4 → l5: five levels exceeds GitHub's four.
	files := map[string]string{}
	for i := 1; i <= 5; i++ {
		var body string
		if i == 5 {
			body = "name: l5\non:\n  workflow_call:\njobs:\n  leaf:\n    steps:\n      - run: echo leaf\n"
		} else {
			body = "name: l" + string(rune('0'+i)) + "\non:\n  workflow_call:\njobs:\n  next:\n    uses: ./.github/workflows/l" + string(rune('0'+i+1)) + ".yml\n"
		}
		files[".github/workflows/l"+string(rune('0'+i))+".yml"] = body
	}
	commitFilesToStorage(t, testServer, repoKey, files)

	def := &WorkflowDef{Name: "deep", Jobs: map[string]*JobDef{
		"start": {Uses: "./.github/workflows/l2.yml"},
	}}
	meta := &WorkflowEventMeta{EventName: "push", Repo: repoKey}
	_, err := testServer.submitWorkflow(context.Background(), "http://localhost", def, "alpine:latest", meta)
	if err == nil || !strings.Contains(err.Error(), "nested deeper") {
		t.Errorf("err = %v, want nesting-depth error", err)
	}
}

func TestRemapCallSecrets(t *testing.T) {
	binding := &WorkflowCallBinding{
		CalledPath: "x.yml",
		SecretsMap: map[string]string{
			"deploy-key": "${{ secrets.PROD_KEY }}",
		},
	}
	wf := &Workflow{}
	got := remapCallSecrets(testServer, wf, binding, map[string]string{
		"PROD_KEY": "sekrit",
		"OTHER":    "hidden-from-called",
	})
	if got["deploy-key"] != "sekrit" {
		t.Errorf("deploy-key = %q, want mapped from PROD_KEY", got["deploy-key"])
	}
	if _, leaked := got["OTHER"]; leaked {
		t.Error("unmapped caller secret must not pass through")
	}
	if len(got) != 1 {
		t.Errorf("got %d secrets, want 1", len(got))
	}
}
