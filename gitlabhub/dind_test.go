package gitlabhub

import "testing"

func TestIsDinDService(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"docker:dind", true},
		{"docker:24.0-dind", true},
		{"docker:latest-dind", true},
		{"redis:7", false},
		{"docker:latest", false},
		{"postgres:15", false},
	}
	for _, tc := range tests {
		if got := isDinDService(tc.image); got != tc.want {
			t.Errorf("isDinDService(%q) = %v, want %v", tc.image, got, tc.want)
		}
	}
}

func TestDinDVariableInjection(t *testing.T) {
	yaml := `
test:
  services:
    - docker:24.0-dind
  script:
    - docker build .
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if len(job.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(job.Services))
	}
	if !isDinDService(job.Services[0].Name) {
		t.Error("expected DinD service detected")
	}
}

func TestServiceVariablesPassthrough(t *testing.T) {
	yaml := `
test:
  services:
    - name: postgres:15
      variables:
        POSTGRES_DB: testdb
        POSTGRES_USER: runner
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if len(job.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(job.Services))
	}
	svc := job.Services[0]
	if svc.Variables["POSTGRES_DB"] != "testdb" {
		t.Errorf("expected POSTGRES_DB=testdb, got %q", svc.Variables["POSTGRES_DB"])
	}
	if svc.Variables["POSTGRES_USER"] != "runner" {
		t.Errorf("expected POSTGRES_USER=runner, got %q", svc.Variables["POSTGRES_USER"])
	}
}

func TestNonDinDNoInjection(t *testing.T) {
	vars := []VariableDef{
		{Key: "CI", Value: "true", Public: true},
	}
	// No DinD, so injectDinDVars should not be called
	// Just test that isDinDService returns false for non-dind
	if isDinDService("redis:7") {
		t.Error("redis should not be detected as DinD")
	}
	// But verify inject does work when called
	result := injectDinDVars(vars)
	found := false
	for _, v := range result {
		if v.Key == "DOCKER_HOST" {
			found = true
			if v.Value != "tcp://docker:2375" {
				t.Errorf("expected DOCKER_HOST=tcp://docker:2375, got %s", v.Value)
			}
		}
	}
	if !found {
		t.Error("expected DOCKER_HOST to be injected")
	}
}
