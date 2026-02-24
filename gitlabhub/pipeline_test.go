package gitlabhub

import (
	"testing"
)

func TestParseBasicPipeline(t *testing.T) {
	yaml := `
stages:
  - build
  - test

build_job:
  stage: build
  script:
    - echo "building"

test_job:
  stage: test
  script:
    - echo "testing"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(def.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(def.Stages))
	}
	if len(def.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(def.Jobs))
	}

	build := def.Jobs["build_job"]
	if build.Stage != "build" {
		t.Fatalf("expected stage=build, got %s", build.Stage)
	}
	if len(build.Script) != 1 {
		t.Fatalf("expected 1 script line, got %d", len(build.Script))
	}
}

func TestParseDefaultStages(t *testing.T) {
	yaml := `
test_job:
  script:
    - echo "testing"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(def.Stages) != 3 {
		t.Fatalf("expected 3 default stages, got %d", len(def.Stages))
	}
	if def.Stages[0] != "build" || def.Stages[1] != "test" || def.Stages[2] != "deploy" {
		t.Fatalf("unexpected default stages: %v", def.Stages)
	}

	// Default stage for jobs without explicit stage is "test"
	job := def.Jobs["test_job"]
	if job.Stage != "test" {
		t.Fatalf("expected default stage=test, got %s", job.Stage)
	}
}

func TestParseMultiStagePipeline(t *testing.T) {
	yaml := `
stages:
  - build
  - test
  - deploy

compile:
  stage: build
  script:
    - gcc -o app main.c

unit_test:
  stage: test
  script:
    - ./app --test

integration_test:
  stage: test
  script:
    - ./app --integration

deploy_prod:
  stage: deploy
  script:
    - deploy.sh
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(def.Jobs) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(def.Jobs))
	}

	// Check stages are assigned correctly
	if def.Jobs["compile"].Stage != "build" {
		t.Fatalf("compile stage: %s", def.Jobs["compile"].Stage)
	}
	if def.Jobs["unit_test"].Stage != "test" {
		t.Fatalf("unit_test stage: %s", def.Jobs["unit_test"].Stage)
	}
	if def.Jobs["deploy_prod"].Stage != "deploy" {
		t.Fatalf("deploy_prod stage: %s", def.Jobs["deploy_prod"].Stage)
	}
}

func TestParseDAGNeeds(t *testing.T) {
	yaml := `
stages:
  - build
  - test

build:
  stage: build
  script:
    - echo build

test:
  stage: test
  needs: [build]
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	testJob := def.Jobs["test"]
	if len(testJob.Needs) != 1 || testJob.Needs[0] != "build" {
		t.Fatalf("expected needs=[build], got %v", testJob.Needs)
	}
}

func TestParseVariables(t *testing.T) {
	yaml := `
variables:
  GLOBAL_VAR: "global-value"
  SHARED: "from-global"

test:
  variables:
    JOB_VAR: "job-value"
    SHARED: "from-job"
  script:
    - echo $GLOBAL_VAR $JOB_VAR $SHARED
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if def.Variables["GLOBAL_VAR"] != "global-value" {
		t.Fatalf("expected global GLOBAL_VAR=global-value, got %s", def.Variables["GLOBAL_VAR"])
	}

	job := def.Jobs["test"]
	// Job-level variables include merged global vars
	if job.Variables["GLOBAL_VAR"] != "global-value" {
		t.Fatalf("job should inherit GLOBAL_VAR, got %s", job.Variables["GLOBAL_VAR"])
	}
	if job.Variables["JOB_VAR"] != "job-value" {
		t.Fatalf("expected JOB_VAR=job-value, got %s", job.Variables["JOB_VAR"])
	}
	if job.Variables["SHARED"] != "from-job" {
		t.Fatalf("expected SHARED=from-job (job override), got %s", job.Variables["SHARED"])
	}
}

func TestParseServices(t *testing.T) {
	yaml := `
test:
  services:
    - postgres:15
    - name: redis:7-alpine
      alias: cache
  script:
    - echo "with services"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	job := def.Jobs["test"]
	if len(job.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(job.Services))
	}
	if job.Services[0].Name != "postgres:15" {
		t.Fatalf("expected postgres:15, got %s", job.Services[0].Name)
	}
	if job.Services[1].Alias != "cache" {
		t.Fatalf("expected alias=cache, got %s", job.Services[1].Alias)
	}
}

func TestParseArtifacts(t *testing.T) {
	yaml := `
build:
  script:
    - make build
  artifacts:
    paths:
      - build/
      - dist/
    expire_in: 1 hour
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	job := def.Jobs["build"]
	if job.Artifacts == nil {
		t.Fatal("expected artifacts definition")
	}
	if len(job.Artifacts.Paths) != 2 {
		t.Fatalf("expected 2 artifact paths, got %d", len(job.Artifacts.Paths))
	}
	if job.Artifacts.ExpireIn != "1 hour" {
		t.Fatalf("expected expire_in='1 hour', got '%s'", job.Artifacts.ExpireIn)
	}
}

func TestParseRules(t *testing.T) {
	yaml := `
deploy:
  stage: deploy
  rules:
    - if: $CI_PIPELINE_SOURCE == "push"
      when: on_success
    - when: never
  script:
    - deploy.sh
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	job := def.Jobs["deploy"]
	if len(job.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(job.Rules))
	}
	if job.Rules[0].If == "" {
		t.Fatal("expected first rule to have if condition")
	}
	if job.Rules[1].When != "never" {
		t.Fatalf("expected second rule when=never, got %s", job.Rules[1].When)
	}
}

func TestParseAllowFailure(t *testing.T) {
	yaml := `
lint:
  allow_failure: true
  script:
    - lint.sh
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !def.Jobs["lint"].AllowFailure {
		t.Fatal("expected allow_failure=true")
	}
}

func TestParseGlobalVarsOverride(t *testing.T) {
	yaml := `
variables:
  VAR1: global
  VAR2: global

job1:
  variables:
    VAR2: override
  script:
    - echo $VAR1 $VAR2

job2:
  script:
    - echo $VAR1 $VAR2
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// job1 overrides VAR2
	if def.Jobs["job1"].Variables["VAR2"] != "override" {
		t.Fatalf("expected job1 VAR2=override, got %s", def.Jobs["job1"].Variables["VAR2"])
	}
	// job2 inherits global VAR2
	if def.Jobs["job2"].Variables["VAR2"] != "global" {
		t.Fatalf("expected job2 VAR2=global, got %s", def.Jobs["job2"].Variables["VAR2"])
	}
}

func TestParseNoJobs(t *testing.T) {
	yaml := `
stages:
  - build
`
	_, err := ParsePipeline([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for pipeline with no jobs")
	}
}

func TestParseGlobalImage(t *testing.T) {
	yaml := `
image: ruby:3.2

test:
  script:
    - bundle exec rspec
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if def.Image != "ruby:3.2" {
		t.Fatalf("expected global image=ruby:3.2, got %s", def.Image)
	}
}

func TestParseBeforeAfterScript(t *testing.T) {
	yaml := `
test:
  before_script:
    - apt-get update
  script:
    - make test
  after_script:
    - cleanup.sh
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	job := def.Jobs["test"]
	if len(job.BeforeScript) != 1 || job.BeforeScript[0] != "apt-get update" {
		t.Fatalf("unexpected before_script: %v", job.BeforeScript)
	}
	if len(job.AfterScript) != 1 || job.AfterScript[0] != "cleanup.sh" {
		t.Fatalf("unexpected after_script: %v", job.AfterScript)
	}
}

func TestParseCache(t *testing.T) {
	yaml := `
test:
  cache:
    key: gems
    paths:
      - vendor/
    policy: pull-push
  script:
    - bundle install
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	job := def.Jobs["test"]
	if job.Cache == nil {
		t.Fatal("expected cache definition")
	}
	if job.Cache.Key != "gems" {
		t.Fatalf("expected cache key=gems, got %s", job.Cache.Key)
	}
	if len(job.Cache.Paths) != 1 || job.Cache.Paths[0] != "vendor/" {
		t.Fatalf("expected cache paths=[vendor/], got %v", job.Cache.Paths)
	}
}

func TestParseDotPrefixIgnored(t *testing.T) {
	yaml := `
.template:
  script:
    - echo template

test:
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(def.Jobs) != 1 {
		t.Fatalf("expected 1 job (dot-prefix ignored), got %d", len(def.Jobs))
	}
	if _, ok := def.Jobs[".template"]; ok {
		t.Fatal("dot-prefix job should be ignored")
	}
}
