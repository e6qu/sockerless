package gitlabhub

import (
	"strings"
	"testing"
)

func TestExtendsBasic(t *testing.T) {
	yamlData := `
.template:
  image: golang:1.24
  before_script:
    - echo "setup"

test:
  extends: .template
  script:
    - echo "test"
`
	def, err := ParsePipeline([]byte(yamlData))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if job == nil {
		t.Fatal("expected job 'test' to exist")
	}
	if job.Image != "golang:1.24" {
		t.Errorf("expected image golang:1.24, got %s", job.Image)
	}
	// script should be replaced (not merged), so only "echo test"
	if len(job.Script) != 1 || job.Script[0] != `echo "test"` {
		t.Errorf("unexpected script: %v", job.Script)
	}
	// before_script inherited from template
	if len(job.BeforeScript) != 1 || job.BeforeScript[0] != `echo "setup"` {
		t.Errorf("expected inherited before_script, got %v", job.BeforeScript)
	}
}

func TestExtendsChain(t *testing.T) {
	yamlData := `
.base:
  image: alpine:3.19
  variables:
    BASE_VAR: base

.middle:
  extends: .base
  stage: build
  variables:
    MIDDLE_VAR: middle

job:
  extends: .middle
  script:
    - echo "hello"
`
	def, err := ParsePipeline([]byte(yamlData))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["job"]
	if job == nil {
		t.Fatal("expected job 'job' to exist")
	}
	// Image should come from .base via chain
	if job.Image != "alpine:3.19" {
		t.Errorf("expected image alpine:3.19, got %s", job.Image)
	}
	// Stage should come from .middle
	if job.Stage != "build" {
		t.Errorf("expected stage build, got %s", job.Stage)
	}
	// Variables should be merged from the whole chain
	if job.Variables["BASE_VAR"] != "base" {
		t.Errorf("expected BASE_VAR=base, got %s", job.Variables["BASE_VAR"])
	}
	if job.Variables["MIDDLE_VAR"] != "middle" {
		t.Errorf("expected MIDDLE_VAR=middle, got %s", job.Variables["MIDDLE_VAR"])
	}
}

func TestExtendsMultiple(t *testing.T) {
	yamlData := `
.a:
  image: ruby:3.2
  variables:
    FROM_A: "yes"
    SHARED: "from-a"

.b:
  image: python:3.12
  variables:
    FROM_B: "yes"
    SHARED: "from-b"

job:
  extends:
    - .a
    - .b
  script:
    - echo "test"
`
	def, err := ParsePipeline([]byte(yamlData))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["job"]
	if job == nil {
		t.Fatal("expected job 'job' to exist")
	}
	// .b is applied after .a, so image should be from .b
	if job.Image != "python:3.12" {
		t.Errorf("expected image python:3.12, got %s", job.Image)
	}
	// Variables from both templates, SHARED overridden by .b
	if job.Variables["FROM_A"] != "yes" {
		t.Errorf("expected FROM_A=yes, got %s", job.Variables["FROM_A"])
	}
	if job.Variables["FROM_B"] != "yes" {
		t.Errorf("expected FROM_B=yes, got %s", job.Variables["FROM_B"])
	}
	if job.Variables["SHARED"] != "from-b" {
		t.Errorf("expected SHARED=from-b, got %s", job.Variables["SHARED"])
	}
}

func TestExtendsCircular(t *testing.T) {
	yamlData := `
.a:
  extends: .b
  script:
    - echo a

.b:
  extends: .a
  script:
    - echo b

job:
  extends: .a
  script:
    - echo job
`
	_, err := ParsePipeline([]byte(yamlData))
	if err == nil {
		t.Fatal("expected error for circular extends")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular error, got: %v", err)
	}
}

func TestExtendsVariablesMerge(t *testing.T) {
	yamlData := `
variables:
  GLOBAL: global-val

.template:
  variables:
    TMPL_VAR: tmpl-val
    SHARED: from-template

job:
  extends: .template
  variables:
    JOB_VAR: job-val
    SHARED: from-job
  script:
    - echo "test"
`
	def, err := ParsePipeline([]byte(yamlData))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["job"]
	if job == nil {
		t.Fatal("expected job 'job' to exist")
	}
	// Template variable inherited
	if job.Variables["TMPL_VAR"] != "tmpl-val" {
		t.Errorf("expected TMPL_VAR=tmpl-val, got %s", job.Variables["TMPL_VAR"])
	}
	// Job variable present
	if job.Variables["JOB_VAR"] != "job-val" {
		t.Errorf("expected JOB_VAR=job-val, got %s", job.Variables["JOB_VAR"])
	}
	// Job overrides template
	if job.Variables["SHARED"] != "from-job" {
		t.Errorf("expected SHARED=from-job, got %s", job.Variables["SHARED"])
	}
	// Global variable also merged
	if job.Variables["GLOBAL"] != "global-val" {
		t.Errorf("expected GLOBAL=global-val, got %s", job.Variables["GLOBAL"])
	}
}
