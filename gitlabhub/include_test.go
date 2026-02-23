package gitlabhub

import (
	"testing"

	"github.com/go-git/go-git/v5/storage/memory"
)

func createTestRepoWithFiles(t *testing.T, files map[string]string) *memory.Storage {
	t.Helper()
	s := newTestServer(t)
	_, err := s.createProjectRepo("include-test", files)
	if err != nil {
		t.Fatal(err)
	}
	return s.store.GetGitStorage("include-test")
}

func TestIncludeLocalBasic(t *testing.T) {
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": `
include:
  - local: "jobs.yml"
test:
  script:
    - echo test
`,
		"jobs.yml": `
build:
  stage: build
  script:
    - echo build
`,
	})

	mainYAML := []byte(`
include:
  - local: "jobs.yml"
test:
  script:
    - echo test
`)
	merged, err := ResolveIncludes(mainYAML, stor)
	if err != nil {
		t.Fatal(err)
	}

	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := def.Jobs["build"]; !ok {
		t.Error("expected 'build' job from included file")
	}
	if _, ok := def.Jobs["test"]; !ok {
		t.Error("expected 'test' job from main file")
	}
}

func TestIncludeStringForm(t *testing.T) {
	// include: "file.yml" (string form)
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": `include: "extra.yml"`,
		"extra.yml": `
deploy:
  script:
    - echo deploy
`,
	})
	merged, err := ResolveIncludes([]byte(`include: "extra.yml"
test:
  script:
    - echo test
`), stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := def.Jobs["deploy"]; !ok {
		t.Error("expected 'deploy' job from included file")
	}
}

func TestIncludeList(t *testing.T) {
	// Multiple includes
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "include: []",
		"a.yml": `
job_a:
  script: [echo a]
`,
		"b.yml": `
job_b:
  script: [echo b]
`,
	})
	merged, err := ResolveIncludes([]byte(`
include:
  - local: "a.yml"
  - local: "b.yml"
test:
  script: [echo test]
`), stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(def.Jobs))
	}
}

func TestIncludeMissingFile(t *testing.T) {
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "test:\n  script: [echo]",
	})
	_, err := ResolveIncludes([]byte(`
include:
  - local: "nonexistent.yml"
test:
  script: [echo]
`), stor)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestIncludeMergesVariables(t *testing.T) {
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "",
		"vars.yml": `
variables:
  FROM_INCLUDE: "yes"
  SHARED: "from-include"
`,
	})
	merged, err := ResolveIncludes([]byte(`
include:
  - local: "vars.yml"
variables:
  SHARED: "from-main"
  MAIN_ONLY: "yes"
test:
  script: [echo]
`), stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	// Main overrides included
	if def.Variables["SHARED"] != "from-main" {
		t.Errorf("SHARED: expected from-main, got %s", def.Variables["SHARED"])
	}
	if def.Variables["FROM_INCLUDE"] != "yes" {
		t.Errorf("FROM_INCLUDE: expected yes, got %s", def.Variables["FROM_INCLUDE"])
	}
	if def.Variables["MAIN_ONLY"] != "yes" {
		t.Errorf("MAIN_ONLY: expected yes, got %s", def.Variables["MAIN_ONLY"])
	}
}

func TestIncludeMergesStages(t *testing.T) {
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "",
		"stages.yml": `
stages:
  - prepare
  - build
`,
	})
	merged, err := ResolveIncludes([]byte(`
include:
  - local: "stages.yml"
stages:
  - build
  - test
  - deploy
test:
  script: [echo]
`), stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	// Union: prepare, build, test, deploy
	expected := []string{"prepare", "build", "test", "deploy"}
	if len(def.Stages) != len(expected) {
		t.Fatalf("expected %d stages, got %d: %v", len(expected), len(def.Stages), def.Stages)
	}
	for i, s := range expected {
		if def.Stages[i] != s {
			t.Errorf("stage[%d]: expected %s, got %s", i, s, def.Stages[i])
		}
	}
}

func TestIncludeNilStorage(t *testing.T) {
	// When git storage is nil, should return YAML unchanged
	input := []byte(`test:
  script: [echo]
`)
	result, err := ResolveIncludes(input, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should be able to parse the result
	def, err := ParsePipeline(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := def.Jobs["test"]; !ok {
		t.Error("expected 'test' job")
	}
}

func TestIncludeNoIncludeKey(t *testing.T) {
	// When there is no include: key, should return YAML unchanged
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "test:\n  script: [echo]",
	})
	input := []byte(`test:
  script: [echo]
`)
	result, err := ResolveIncludes(input, stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := def.Jobs["test"]; !ok {
		t.Error("expected 'test' job")
	}
}

func TestIncludeTemplateForExtends(t *testing.T) {
	// Include a file with a template, then extend it in the main file
	stor := createTestRepoWithFiles(t, map[string]string{
		".gitlab-ci.yml": "",
		"templates.yml": `
.base:
  image: alpine:latest
  before_script:
    - echo setup
`,
	})
	merged, err := ResolveIncludes([]byte(`
include:
  - local: "templates.yml"
test:
  extends: .base
  script: [echo test]
`), stor)
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParsePipeline(merged)
	if err != nil {
		t.Fatal(err)
	}
	job, ok := def.Jobs["test"]
	if !ok {
		t.Fatal("expected 'test' job")
	}
	if job.Image != "alpine:latest" {
		t.Errorf("expected image alpine:latest, got %s", job.Image)
	}
	if len(job.BeforeScript) == 0 || job.BeforeScript[0] != "echo setup" {
		t.Errorf("expected before_script from template, got %v", job.BeforeScript)
	}
}
