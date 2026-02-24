package gitlabhub

import "testing"

func TestParallelSimple(t *testing.T) {
	yaml := `
test:
  parallel: 3
  script:
    - echo "test"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(def.Jobs))
	}
	// Check job names: "test 1/3", "test 2/3", "test 3/3"
	for _, name := range []string{"test 1/3", "test 2/3", "test 3/3"} {
		if _, ok := def.Jobs[name]; !ok {
			t.Errorf("missing job %q", name)
		}
	}
}

func TestMatrixExpansion(t *testing.T) {
	yaml := `
test:
  parallel:
    matrix:
      - RUBY: ["3.0", "3.1"]
        DB: ["pg", "mysql"]
  script:
    - echo "$RUBY $DB"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Jobs) != 4 {
		t.Fatalf("expected 4 jobs, got %d: %v", len(def.Jobs), jobNames(def))
	}
}

func TestMatrix3Way(t *testing.T) {
	// 3 values x 2 values = 6 jobs
	yaml := `
test:
  parallel:
    matrix:
      - OS: ["linux", "mac", "win"]
        VER: ["1", "2"]
  script:
    - echo "$OS $VER"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Jobs) != 6 {
		t.Fatalf("expected 6 jobs, got %d", len(def.Jobs))
	}
}

func TestParallelPreservesStage(t *testing.T) {
	yaml := `
stages:
  - test
job:
  stage: test
  parallel: 2
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range def.Jobs {
		if j.Stage != "test" {
			t.Errorf("expected stage test, got %s", j.Stage)
		}
	}
}

func TestParallelPreservesNeeds(t *testing.T) {
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
  parallel: 2
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	for name, j := range def.Jobs {
		if j.MatrixGroup == "test" {
			if len(j.Needs) != 1 || j.Needs[0] != "build" {
				t.Errorf("job %s: expected needs [build], got %v", name, j.Needs)
			}
		}
	}
}

func TestMatrixVariableInjection(t *testing.T) {
	yaml := `
test:
  parallel:
    matrix:
      - DB: ["pg"]
  script:
    - echo "$DB"
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range def.Jobs {
		if j.Variables["DB"] != "pg" {
			t.Errorf("expected DB=pg, got %s", j.Variables["DB"])
		}
	}
}

func TestNoExpansionWithoutKeyword(t *testing.T) {
	yaml := `
test:
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(def.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(def.Jobs))
	}
}

func TestMatrixDisplayNames(t *testing.T) {
	yaml := `
test:
  parallel:
    matrix:
      - A: ["x"]
        B: ["y"]
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	// Should have "test (x, y)" (sorted keys: A, B)
	if _, ok := def.Jobs["test (x, y)"]; !ok {
		t.Errorf("expected job named 'test (x, y)', got: %v", jobNames(def))
	}
}

func jobNames(def *PipelineDef) []string {
	var names []string
	for k := range def.Jobs {
		names = append(names, k)
	}
	return names
}
