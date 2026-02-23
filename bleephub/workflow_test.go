package bleephub

import (
	"testing"
)

func TestWorkflowParseSingleJob(t *testing.T) {
	yaml := `
name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
      - run: echo world
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if wf.Name != "CI" {
		t.Errorf("name = %q, want CI", wf.Name)
	}
	job, ok := wf.Jobs["test"]
	if !ok {
		t.Fatal("missing job 'test'")
	}
	if len(job.Steps) != 2 {
		t.Errorf("steps = %d, want 2", len(job.Steps))
	}
	if job.Steps[0].Run != "echo hello" {
		t.Errorf("step[0].run = %q", job.Steps[0].Run)
	}
}

func TestWorkflowParseMultiJobWithNeeds(t *testing.T) {
	yaml := `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make build
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: make test
  deploy:
    needs: [build, test]
    runs-on: ubuntu-latest
    steps:
      - run: make deploy
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(wf.Jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(wf.Jobs))
	}

	// needs as string
	testJob := wf.Jobs["test"]
	if len(testJob.Needs) != 1 || testJob.Needs[0] != "build" {
		t.Errorf("test.needs = %v, want [build]", testJob.Needs)
	}

	// needs as list
	deployJob := wf.Jobs["deploy"]
	if len(deployJob.Needs) != 2 {
		t.Errorf("deploy.needs = %v, want [build, test]", deployJob.Needs)
	}
}

func TestWorkflowParseUsesSteps(t *testing.T) {
	yaml := `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: go test ./...
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["build"]
	if job.Steps[0].Uses != "actions/checkout@v4" {
		t.Errorf("step[0].uses = %q", job.Steps[0].Uses)
	}
	if job.Steps[1].Uses != "actions/setup-go@v5" {
		t.Errorf("step[1].uses = %q", job.Steps[1].Uses)
	}
	if job.Steps[1].With["go-version"] != "1.22" {
		t.Errorf("step[1].with = %v", job.Steps[1].With)
	}
	if job.Steps[2].Run != "go test ./..." {
		t.Errorf("step[2].run = %q", job.Steps[2].Run)
	}
}

func TestWorkflowParseMatrixStrategy(t *testing.T) {
	yaml := `
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        go: ["1.21", "1.22"]
        include:
          - os: ubuntu-latest
            go: "1.23"
        exclude:
          - os: macos-latest
            go: "1.21"
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo test
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.Strategy == nil {
		t.Fatal("strategy is nil")
	}
	m := job.Strategy.Matrix
	if len(m.Values["os"]) != 2 {
		t.Errorf("matrix.os = %v, want 2 values", m.Values["os"])
	}
	if len(m.Values["go"]) != 2 {
		t.Errorf("matrix.go = %v, want 2 values", m.Values["go"])
	}
	if len(m.Include) != 1 {
		t.Errorf("matrix.include = %v, want 1", m.Include)
	}
	if len(m.Exclude) != 1 {
		t.Errorf("matrix.exclude = %v, want 1", m.Exclude)
	}
}

func TestWorkflowParseContainerAsString(t *testing.T) {
	yaml := `
jobs:
  test:
    container: node:18
    steps:
      - run: node --version
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.ContainerImage() != "node:18" {
		t.Errorf("container image = %q, want node:18", job.ContainerImage())
	}
}

func TestWorkflowParseContainerAsObject(t *testing.T) {
	yaml := `
jobs:
  test:
    container:
      image: node:18
      env:
        NODE_ENV: test
    steps:
      - run: node --version
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.ContainerImage() != "node:18" {
		t.Errorf("container image = %q, want node:18", job.ContainerImage())
	}
}

func TestWorkflowParseInvalidYAML(t *testing.T) {
	_, err := ParseWorkflow([]byte(`{invalid yaml`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestWorkflowParseNoJobs(t *testing.T) {
	_, err := ParseWorkflow([]byte(`name: empty`))
	if err == nil {
		t.Fatal("expected error for empty jobs")
	}
}

func TestWorkflowParseEnv(t *testing.T) {
	yaml := `
jobs:
  test:
    env:
      FOO: bar
    steps:
      - run: echo $FOO
        env:
          BAZ: qux
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.Env["FOO"] != "bar" {
		t.Errorf("job.env = %v", job.Env)
	}
	if job.Steps[0].Env["BAZ"] != "qux" {
		t.Errorf("step.env = %v", job.Steps[0].Env)
	}
}

func TestWorkflowParseJobOutputs(t *testing.T) {
	yaml := `
jobs:
  build:
    outputs:
      version: ${{ steps.ver.outputs.version }}
    steps:
      - id: ver
        run: echo "version=1.0" >> $GITHUB_OUTPUT
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["build"]
	if job.Outputs["version"] != "${{ steps.ver.outputs.version }}" {
		t.Errorf("outputs = %v", job.Outputs)
	}
}

func TestParseActionRef(t *testing.T) {
	tests := []struct {
		uses            string
		wantNWO         string
		wantPath        string
		wantRef         string
		wantLocal       bool
	}{
		{"actions/checkout@v4", "actions/checkout", "", "v4", false},
		{"actions/setup-go@v5", "actions/setup-go", "", "v5", false},
		{"owner/repo/subdir@main", "owner/repo", "subdir", "main", false},
		{"./local/path", "", "./local/path", "", true},
		{"../sibling", "", "../sibling", "", true},
	}

	for _, tc := range tests {
		nwo, path, ref, isLocal := ParseActionRef(tc.uses)
		if nwo != tc.wantNWO || path != tc.wantPath || ref != tc.wantRef || isLocal != tc.wantLocal {
			t.Errorf("ParseActionRef(%q) = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
				tc.uses, nwo, path, ref, isLocal,
				tc.wantNWO, tc.wantPath, tc.wantRef, tc.wantLocal)
		}
	}
}

func TestParseWorkflowServices(t *testing.T) {
	yaml := `
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if len(job.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(job.Services))
	}
	redis := job.Services["redis"]
	if redis == nil {
		t.Fatal("redis service is nil")
	}
	if redis.Image != "redis:7" {
		t.Errorf("redis.image = %q, want redis:7", redis.Image)
	}
}

func TestParseWorkflowServicesStringImage(t *testing.T) {
	yaml := `
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      redis: redis:7
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	redis := job.Services["redis"]
	if redis == nil {
		t.Fatal("redis service is nil")
	}
	if redis.Image != "redis:7" {
		t.Errorf("redis.image = %q, want redis:7", redis.Image)
	}
}

func TestParseWorkflowServicesWithEnvPorts(t *testing.T) {
	yaml := `
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
        ports:
          - 5432:5432
        volumes:
          - /data:/var/lib/postgresql/data
        options: --health-cmd pg_isready
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	pg := job.Services["postgres"]
	if pg == nil {
		t.Fatal("postgres service is nil")
	}
	if pg.Image != "postgres:15" {
		t.Errorf("image = %q, want postgres:15", pg.Image)
	}
	if pg.Env["POSTGRES_PASSWORD"] != "test" {
		t.Errorf("env = %v", pg.Env)
	}
	if len(pg.Ports) != 1 {
		t.Errorf("ports = %v, want 1 entry", pg.Ports)
	}
	if len(pg.Volumes) != 1 || pg.Volumes[0] != "/data:/var/lib/postgresql/data" {
		t.Errorf("volumes = %v", pg.Volumes)
	}
	if pg.Options != "--health-cmd pg_isready" {
		t.Errorf("options = %q", pg.Options)
	}
}

func TestParseWorkflowNoServices(t *testing.T) {
	yaml := `
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.Services != nil {
		t.Errorf("services should be nil, got %v", job.Services)
	}
}

func TestWorkflowParseStrategyFailFast(t *testing.T) {
	yaml := `
jobs:
  test:
    strategy:
      fail-fast: false
      max-parallel: 2
      matrix:
        os: [ubuntu-latest]
    steps:
      - run: echo test
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := wf.Jobs["test"]
	if job.Strategy.FailFast == nil || *job.Strategy.FailFast != false {
		t.Error("fail-fast should be false")
	}
	if job.Strategy.MaxParallel != 2 {
		t.Errorf("max-parallel = %d, want 2", job.Strategy.MaxParallel)
	}
}
