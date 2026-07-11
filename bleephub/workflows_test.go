package bleephub

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkflowSingleJobSubmit(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{{Run: "echo hello"}},
			},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if workflow.Status != "running" {
		t.Errorf("status = %q, want running", workflow.Status)
	}
	if len(workflow.Jobs) != 1 {
		t.Errorf("jobs = %d, want 1", len(workflow.Jobs))
	}
	job := workflow.Jobs["build"]
	if job.Status != "queued" {
		t.Errorf("job status = %q, want queued", job.Status)
	}
}

func TestInternalSubmitJobRequiresExplicitImageOrHostMode(t *testing.T) {
	s := newTestServer()
	s.registerJobRoutes()

	req := httptest.NewRequest(http.MethodPost, "/internal/exec/submit", bytes.NewBufferString(`{"steps":[{"run":"echo hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	if len(s.store.Jobs) != 0 {
		t.Fatalf("queued %d jobs for invalid submission", len(s.store.Jobs))
	}
}

func TestInternalSubmitWorkflowRequiresExplicitImageOrHostMode(t *testing.T) {
	s := newTestServer()
	s.registerJobRoutes()

	body := `{"workflow":"name: explicit\njobs:\n  test:\n    runs-on: self-hosted\n    steps:\n      - run: echo hello\n"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/exec/workflow", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	if len(s.store.Workflows) != 0 {
		t.Fatalf("queued %d workflows for invalid submission", len(s.store.Workflows))
	}
}

func TestWorkflowTwoJobsWithNeeds(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{{Run: "make build"}},
			},
			"test": {
				Needs: []string{"build"},
				Steps: []StepDef{{Run: "make test"}},
			},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// build should be dispatched, test should be pending
	buildJob := workflow.Jobs["build"]
	testJob := workflow.Jobs["test"]

	if buildJob.Status != "queued" {
		t.Errorf("build status = %q, want queued", buildJob.Status)
	}
	if testJob.Status != "pending" {
		t.Errorf("test status = %q, want pending", testJob.Status)
	}

	// Simulate build completion — store serverURL in env for re-dispatch
	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}
	s.onJobCompleted(context.Background(), buildJob.JobID, "Succeeded")

	if testJob.Status != "queued" {
		t.Errorf("test status after build = %q, want queued", testJob.Status)
	}
}

func TestWorkflowDiamondDependency(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "diamond",
		Jobs: map[string]*JobDef{
			"a": {Steps: []StepDef{{Run: "echo a"}}},
			"b": {Needs: []string{"a"}, Steps: []StepDef{{Run: "echo b"}}},
			"c": {Needs: []string{"a"}, Steps: []StepDef{{Run: "echo c"}}},
			"d": {Needs: []string{"b", "c"}, Steps: []StepDef{{Run: "echo d"}}},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}

	// Only A should be dispatched
	if workflow.Jobs["a"].Status != "queued" {
		t.Errorf("a = %q, want queued", workflow.Jobs["a"].Status)
	}
	if workflow.Jobs["b"].Status != "pending" {
		t.Errorf("b = %q, want pending", workflow.Jobs["b"].Status)
	}
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete A → B and C should dispatch
	s.onJobCompleted(context.Background(), workflow.Jobs["a"].JobID, "Succeeded")
	if workflow.Jobs["b"].Status != "queued" {
		t.Errorf("b after a = %q, want queued", workflow.Jobs["b"].Status)
	}
	if workflow.Jobs["c"].Status != "queued" {
		t.Errorf("c after a = %q, want queued", workflow.Jobs["c"].Status)
	}
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d after a = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete B → D still pending (C not done)
	s.onJobCompleted(context.Background(), workflow.Jobs["b"].JobID, "Succeeded")
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d after b = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete C → D dispatches, workflow complete
	s.onJobCompleted(context.Background(), workflow.Jobs["c"].JobID, "Succeeded")
	if workflow.Jobs["d"].Status != "queued" {
		t.Errorf("d after c = %q, want queued", workflow.Jobs["d"].Status)
	}
}

func TestWorkflowFailedJobSkipsDependents(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "fail-test",
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "exit 1"}}},
			"test":  {Needs: []string{"build"}, Steps: []StepDef{{Run: "echo test"}}},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}

	// Build fails
	s.onJobCompleted(context.Background(), workflow.Jobs["build"].JobID, "Failed")

	if workflow.Jobs["test"].Status != "skipped" {
		t.Errorf("test status = %q, want skipped", workflow.Jobs["test"].Status)
	}
	if workflow.Status != "completed" {
		t.Errorf("workflow status = %q, want completed", workflow.Status)
	}
	if workflow.Result != "failure" {
		t.Errorf("workflow result = %q, want failure", workflow.Result)
	}
}

func TestWorkflowNeedsContextPropagation(t *testing.T) {
	wf := &Workflow{
		ID:   "test-wf",
		Name: "test",
		Jobs: map[string]*WorkflowJob{
			"build": {
				Key:    "build",
				JobID:  "j1",
				Status: "completed",
				Result: "success",
				Outputs: map[string]string{
					"version": "1.0.0",
				},
			},
			"deploy": {
				Key:   "deploy",
				JobID: "j2",
				Needs: []string{"build"},
			},
		},
	}

	ctx := buildNeedsContext(wf, wf.Jobs["deploy"])
	dict, ok := ctx.(map[string]interface{})
	if !ok {
		t.Fatalf("needs context is not a dict: %T", ctx)
	}
	if dict["t"] != 2 {
		t.Errorf("needs context type = %v, want 2", dict["t"])
	}
	entries, ok := dict["d"].([]map[string]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("needs context entries = %v", dict["d"])
	}
	if entries[0]["k"] != "build" {
		t.Errorf("needs[0].k = %v, want build", entries[0]["k"])
	}
}

func TestWorkflowUsesStepReference(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "uses-test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{
					{Uses: "actions/checkout@v4"},
					{Run: "echo done"},
				},
			},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Verify the job message contains a repository reference
	s.store.mu.RLock()
	job := s.store.Jobs[workflow.Jobs["build"].JobID]
	s.store.mu.RUnlock()

	if job == nil {
		t.Fatal("job not found in store")
	}
	if job.Message == "" {
		t.Fatal("job message is empty")
	}

	// Parse the message and check the first step's reference
	var msg map[string]interface{}
	if err := jsonUnmarshal([]byte(job.Message), &msg); err != nil {
		t.Fatalf("parse message: %v", err)
	}

	steps, ok := msg["steps"].([]interface{})
	if !ok || len(steps) < 1 {
		t.Fatal("no steps in message")
	}

	step0 := steps[0].(map[string]interface{})
	ref := step0["reference"].(map[string]interface{})
	if ref["type"] != "repository" {
		t.Errorf("ref type = %v, want repository", ref["type"])
	}
	if ref["name"] != "actions/checkout" {
		t.Errorf("ref name = %v, want actions/checkout", ref["name"])
	}
	if ref["ref"] != "v4" {
		t.Errorf("ref ref = %v, want v4", ref["ref"])
	}
}

func TestValidateJobGraphCycle(t *testing.T) {
	wf := &WorkflowDef{
		Jobs: map[string]*JobDef{
			"a": {Needs: []string{"b"}},
			"b": {Needs: []string{"a"}},
		},
	}
	err := validateJobGraph(wf)
	if err == nil {
		t.Error("expected cycle error")
	}
}

func TestValidateJobGraphUnknownDep(t *testing.T) {
	wf := &WorkflowDef{
		Jobs: map[string]*JobDef{
			"a": {Needs: []string{"nonexistent"}},
		},
	}
	err := validateJobGraph(wf)
	if err == nil {
		t.Error("expected unknown dependency error")
	}
}

func TestNormalizeResult(t *testing.T) {
	tests := map[string]string{
		"Succeeded": "success",
		"succeeded": "success",
		"Failed":    "failure",
		"failed":    "failure",
		"Cancelled": "cancelled",
		"":          "success",
		"custom":    "custom",
	}
	for input, expected := range tests {
		if got := normalizeResult(input); got != expected {
			t.Errorf("normalizeResult(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestBuildJobMessageWithServices(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "svc-test",
		Jobs: map[string]*JobDef{
			"test": {
				Services: map[string]*ServiceDef{
					"redis": {
						Image: "redis:7",
						Ports: []interface{}{"6379:6379"},
					},
					"postgres": {
						Image: "postgres:15",
						Env:   map[string]string{"POSTGRES_PASSWORD": "test"},
					},
				},
				Steps: []StepDef{{Run: "echo hello"}},
			},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	s.store.mu.RLock()
	job := s.store.Jobs[workflow.Jobs["test"].JobID]
	s.store.mu.RUnlock()

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(job.Message), &msg); err != nil {
		t.Fatalf("parse message: %v", err)
	}

	// The official runner deserializes jobServiceContainers as a
	// mapping TemplateToken ({"type":2,"map":[{Key,Value}]}); a plain
	// JSON map fails its template validation at job start.
	svcContainers, ok := msg["jobServiceContainers"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobServiceContainers is %T, want mapping token", msg["jobServiceContainers"])
	}
	if got := svcContainers["type"]; got != float64(2) {
		t.Fatalf("jobServiceContainers token type = %v, want 2 (mapping)", got)
	}
	entries := tokenMapEntries(t, svcContainers)
	if len(entries) != 2 {
		t.Fatalf("service containers = %d, want 2", len(entries))
	}

	redis := entries["redis"]
	if redis == nil {
		t.Fatalf("redis service missing from mapping token")
	}
	redisSpec := tokenMapEntries(t, redis)
	if got := tokenLit(redisSpec["image"]); got != "redis:7" {
		t.Errorf("redis.image = %v", got)
	}
	ports, ok := redisSpec["ports"].(map[string]interface{})
	if !ok || ports["type"] != float64(1) {
		t.Fatalf("redis.ports = %v, want sequence token", redisSpec["ports"])
	}
	seq := ports["seq"].([]interface{})
	if len(seq) != 1 || tokenLit(seq[0]) != "6379:6379" {
		t.Errorf("redis.ports seq = %v", seq)
	}

	pg := entries["postgres"]
	if pg == nil {
		t.Fatalf("postgres service missing from mapping token")
	}
	pgSpec := tokenMapEntries(t, pg)
	if got := tokenLit(pgSpec["image"]); got != "postgres:15" {
		t.Errorf("postgres.image = %v", got)
	}
	// Env key is `env` (the runner's ContainerInfo reader), not
	// `environment`.
	envTok, ok := pgSpec["env"].(map[string]interface{})
	if !ok {
		t.Fatalf("postgres.env is %T, want mapping token", pgSpec["env"])
	}
	envEntries := tokenMapEntries(t, envTok)
	if got := tokenLit(envEntries["POSTGRES_PASSWORD"]); got != "test" {
		t.Errorf("postgres.env = %v", envEntries)
	}
}

// tokenMapEntries flattens a mapping TemplateToken's {Key,Value} pairs
// into a name → value-token map.
func tokenMapEntries(t *testing.T, tok interface{}) map[string]interface{} {
	t.Helper()
	m, ok := tok.(map[string]interface{})
	if !ok {
		t.Fatalf("token is %T, want map", tok)
	}
	pairs, ok := m["map"].([]interface{})
	if !ok {
		t.Fatalf("mapping token has no map array: %v", m)
	}
	out := make(map[string]interface{}, len(pairs))
	for _, p := range pairs {
		entry := p.(map[string]interface{})
		key, _ := tokenLit(entry["Key"]).(string)
		out[key] = entry["Value"]
	}
	return out
}

// tokenLit extracts the literal value of a string/number/bool token.
func tokenLit(tok interface{}) interface{} {
	m, ok := tok.(map[string]interface{})
	if !ok {
		return nil
	}
	return m["lit"]
}

func TestBuildJobMessageNoServices(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "no-svc",
		Jobs: map[string]*JobDef{
			"test": {
				Steps: []StepDef{{Run: "echo hello"}},
			},
		},
	}

	workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	s.store.mu.RLock()
	job := s.store.Jobs[workflow.Jobs["test"].JobID]
	s.store.mu.RUnlock()

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(job.Message), &msg); err != nil {
		t.Fatalf("parse message: %v", err)
	}

	if msg["jobServiceContainers"] != nil {
		t.Errorf("jobServiceContainers should be nil, got %v", msg["jobServiceContainers"])
	}
}

func TestBuildJobMessageRepoLessGithubContextHasNoFakeRefSha(t *testing.T) {
	req := &SubmitRequest{HostMode: true, Steps: []SubmitStep{{Run: "echo hello"}}}
	msg := buildJobMessage("http://localhost", "job", "plan", "timeline", 1, req)
	ctx := msg["contextData"].(map[string]interface{})
	githubCtx := pipelineContextMap(t, ctx["github"])
	if githubCtx["repository"] != "" || githubCtx["repository_owner"] != "" {
		t.Fatalf("repo-less context repository = %q owner = %q", githubCtx["repository"], githubCtx["repository_owner"])
	}
	if githubCtx["sha"] != "" || githubCtx["ref"] != "" {
		t.Fatalf("repo-less context sha/ref = %q/%q, want empty values", githubCtx["sha"], githubCtx["ref"])
	}
}

func TestGithubContextMapRepoLessHasNoFakeRefSha(t *testing.T) {
	s := newTestServer()
	wf := &Workflow{Name: "operator", RunID: 7, RunNumber: 7}
	ctx := s.githubContextMap(wf)
	if ctx["repository"] != "" || ctx["repository_owner"] != "" {
		t.Fatalf("repo-less context repository = %q owner = %q", ctx["repository"], ctx["repository_owner"])
	}
	if ctx["sha"] != "" || ctx["ref"] != "" {
		t.Fatalf("repo-less context sha/ref = %q/%q, want empty values", ctx["sha"], ctx["ref"])
	}
}

func TestSubmitWorkflowRepoRefResolution(t *testing.T) {
	repo := seedTestRepo(t, "internal-submit-ref", false)
	yaml := "name: internal-submit\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	stor := testServer.store.GetGitStorage("admin", "internal-submit-ref")
	if stor == nil {
		t.Fatalf("git storage for %s missing", repo.FullName)
	}
	admin := testServer.store.Users[1]
	commit, err := initRepoWithFiles(stor, repo.DefaultBranch, "seed workflow", map[string]string{
		".github/workflows/internal-submit.yml": yaml,
	}, repoSignature(admin.Login, "bleephub@local"))
	if err != nil {
		t.Fatalf("seed workflow git state: %v", err)
	}
	wantSha := commit.String()
	body, _ := json.Marshal(map[string]string{
		"workflow": yaml,
		"repo":     repo.FullName,
		"ref":      "refs/heads/main",
		"image":    "alpine:latest",
	})
	resp, err := authedPost("/internal/exec/workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	data := decodeJSONWithStatus(t, resp, 200)
	wfID, _ := data["workflowId"].(string)
	if wfID == "" {
		t.Fatalf("submit response missing workflowId: %v", data)
	}
	testServer.store.mu.RLock()
	wf := testServer.store.Workflows[wfID]
	testServer.store.mu.RUnlock()
	if wf == nil {
		t.Fatalf("workflow %q not stored", wfID)
	}
	if wf.Sha != wantSha {
		t.Fatalf("workflow sha = %q, want resolved git sha %q", wf.Sha, wantSha)
	}
}

func TestSubmitWorkflowRejectsUnresolvedRepoRef(t *testing.T) {
	repo := seedTestRepo(t, "internal-submit-missing-ref", false)
	yaml := "name: missing-ref\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	body, _ := json.Marshal(map[string]string{
		"workflow": yaml,
		"repo":     repo.FullName,
		"ref":      "refs/heads/missing",
		"image":    "alpine:latest",
	})
	resp, err := authedPost("/internal/exec/workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func pipelineContextMap(t *testing.T, ctx interface{}) map[string]string {
	t.Helper()
	m, ok := ctx.(map[string]interface{})
	if !ok {
		t.Fatalf("context is %T, want map", ctx)
	}
	out := make(map[string]string)
	add := func(entry map[string]interface{}) {
		k, _ := entry["k"].(string)
		v, _ := entry["v"].(string)
		out[k] = v
	}
	switch entries := m["d"].(type) {
	case []interface{}:
		for _, raw := range entries {
			entry, ok := raw.(map[string]interface{})
			if !ok {
				t.Fatalf("entry is %T, want map", raw)
			}
			add(entry)
		}
	case []map[string]interface{}:
		for _, entry := range entries {
			add(entry)
		}
	default:
		t.Fatalf("context entries are %T, want slice", m["d"])
	}
	return out
}

// jsonUnmarshal is a test helper.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// A job targeting a reviewer-protected environment must wait for a
// deployment review; approval releases it, rejection fails the run.
func TestWorkflowEnvironmentApprovalGate(t *testing.T) {
	mkServer := func(t *testing.T) (*Server, *Repo, *Workflow, *Environment) {
		t.Helper()
		s := newTestServer()
		admin := s.store.Users[1]
		repo := s.store.CreateRepo(admin, "envgate", "", false)
		if repo == nil {
			t.Fatal("create repo failed")
		}
		env := s.store.Deployments.UpsertEnvironment(repo.ID, "production")
		s.store.Deployments.SetEnvironmentProtection(repo.ID, "production", nil,
			[]map[string]interface{}{{"type": "User", "id": admin.ID}})

		wf := &WorkflowDef{
			Name: "deploy",
			// The HTTP submit path stashes the dispatch wiring in Env;
			// direct submitWorkflow callers carry the same contract.
			Env: map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"},
			Jobs: map[string]*JobDef{
				"release": {
					Environment: "production",
					Steps:       []StepDef{{Run: "echo deploy"}},
				},
			},
		}
		workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest",
			&WorkflowEventMeta{EventName: "push", Repo: repo.FullName, Ref: "refs/heads/main"})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if workflow.Status != WorkflowStatusWaiting {
			t.Fatalf("workflow status = %q, want waiting", workflow.Status)
		}
		if got := workflow.Jobs["release"].Status; got != JobStatusWaiting {
			t.Fatalf("job status = %q, want waiting", got)
		}
		if len(workflow.PendingDeployments) != 1 || workflow.PendingDeployments[0].EnvName != "production" {
			t.Fatalf("pending deployments = %+v", workflow.PendingDeployments)
		}
		return s, repo, workflow, env
	}

	t.Run("approve releases the job", func(t *testing.T) {
		s, _, workflow, env := mkServer(t)
		admin := s.store.Users[1]
		s.applyDeploymentReview(context.Background(), workflow, []int{env.ID}, "approved", "ship it", admin)

		if workflow.Status != WorkflowStatusRunning && workflow.Status != WorkflowStatusCompleted {
			t.Errorf("workflow status = %q, want running", workflow.Status)
		}
		if got := workflow.Jobs["release"].Status; got != JobStatusQueued {
			t.Errorf("job status = %q, want queued after approval", got)
		}
		if len(workflow.PendingDeployments) != 0 {
			t.Errorf("pending deployments not cleared: %+v", workflow.PendingDeployments)
		}
		if len(workflow.EnvApprovals) != 1 || workflow.EnvApprovals[0].State != "approved" {
			t.Errorf("approvals = %+v", workflow.EnvApprovals)
		}
	})

	t.Run("reject fails the run", func(t *testing.T) {
		s, _, workflow, env := mkServer(t)
		admin := s.store.Users[1]
		s.applyDeploymentReview(context.Background(), workflow, []int{env.ID}, "rejected", "not today", admin)

		if workflow.Status != WorkflowStatusCompleted {
			t.Errorf("workflow status = %q, want completed", workflow.Status)
		}
		if workflow.Result != ResultFailure {
			t.Errorf("workflow result = %q, want failure", workflow.Result)
		}
		if got := workflow.Jobs["release"].Result; got != ResultFailure {
			t.Errorf("job result = %q, want failure", got)
		}
	})

	t.Run("environment without reviewers does not gate", func(t *testing.T) {
		s := newTestServer()
		admin := s.store.Users[1]
		repo := s.store.CreateRepo(admin, "envfree", "", false)
		if repo == nil {
			t.Fatal("create repo failed")
		}
		wf := &WorkflowDef{
			Name: "deploy",
			Jobs: map[string]*JobDef{
				"release": {Environment: "staging", Steps: []StepDef{{Run: "echo x"}}},
			},
		}
		workflow, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest",
			&WorkflowEventMeta{EventName: "push", Repo: repo.FullName, Ref: "refs/heads/main"})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if got := workflow.Jobs["release"].Status; got != JobStatusQueued {
			t.Errorf("job status = %q, want queued (no reviewers)", got)
		}
		// Referencing the environment auto-created it, like real GitHub.
		if env := s.store.Deployments.GetEnvironment(repo.ID, "staging"); env == nil {
			t.Errorf("environment was not auto-created")
		}
	})
}
