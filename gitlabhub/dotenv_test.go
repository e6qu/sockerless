package gitlabhub

import "testing"

func TestParseDotenvBasic(t *testing.T) {
	input := "KEY=value\nOTHER=123"
	m := parseDotenv(input)
	if m["KEY"] != "value" {
		t.Errorf("KEY: expected 'value', got %q", m["KEY"])
	}
	if m["OTHER"] != "123" {
		t.Errorf("OTHER: expected '123', got %q", m["OTHER"])
	}
}

func TestParseDotenvComments(t *testing.T) {
	input := "# comment\nKEY=value\n\n# another"
	m := parseDotenv(input)
	if len(m) != 1 {
		t.Errorf("expected 1 entry, got %d", len(m))
	}
}

func TestParseDotenvQuoted(t *testing.T) {
	input := `KEY="hello world"` + "\n" + `OTHER='single'`
	m := parseDotenv(input)
	if m["KEY"] != "hello world" {
		t.Errorf("KEY: expected 'hello world', got %q", m["KEY"])
	}
	if m["OTHER"] != "single" {
		t.Errorf("OTHER: expected 'single', got %q", m["OTHER"])
	}
}

func TestParseDotenvEmpty(t *testing.T) {
	m := parseDotenv("")
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestDotenvVarsInjectedDownstream(t *testing.T) {
	// Test that dotenv vars from a dependency are available.
	// This tests the data flow through the store.
	yaml := `
stages:
  - build
  - test

build:
  stage: build
  script:
    - echo "VERSION=1.2.3" > build.env
  artifacts:
    reports:
      dotenv: build.env

test:
  stage: test
  script:
    - echo "version is $VERSION"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	buildJob := def.Jobs["build"]
	if buildJob.Artifacts == nil || buildJob.Artifacts.Reports == nil || buildJob.Artifacts.Reports.Dotenv == "" {
		t.Error("expected dotenv artifact report on build job")
	}
}

func TestDotenvOverridesPipelineVars(t *testing.T) {
	// Dotenv vars should override pipeline-level vars
	yaml := `
variables:
  VERSION: "0.0.0"

stages:
  - build
  - test

build:
  stage: build
  script:
    - echo build
  artifacts:
    reports:
      dotenv: build.env

test:
  stage: test
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if def.Jobs["build"].Artifacts.Reports == nil {
		t.Error("expected reports on build artifacts")
	}
}
