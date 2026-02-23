package gitlabhub

import "testing"

func TestParseDurationSeconds(t *testing.T) {
	if got := parseGitLabDuration("10s"); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestParseDurationMinutes(t *testing.T) {
	if got := parseGitLabDuration("2m"); got != 120 {
		t.Errorf("expected 120, got %d", got)
	}
}

func TestParseDurationHours(t *testing.T) {
	if got := parseGitLabDuration("1h"); got != 3600 {
		t.Errorf("expected 3600, got %d", got)
	}
}

func TestParseDurationCompound(t *testing.T) {
	if got := parseGitLabDuration("1h 30m"); got != 5400 {
		t.Errorf("expected 5400, got %d", got)
	}
}

func TestParseDurationPlainInt(t *testing.T) {
	if got := parseGitLabDuration("3600"); got != 3600 {
		t.Errorf("expected 3600, got %d", got)
	}
}

func TestParseDurationEmpty(t *testing.T) {
	if got := parseGitLabDuration(""); got != 3600 {
		t.Errorf("expected 3600 default, got %d", got)
	}
}

func TestParseRetryInt(t *testing.T) {
	r := parseRetry(2)
	if r == nil || r.Max != 2 {
		t.Errorf("expected max=2, got %v", r)
	}
}

func TestParseRetryClamp(t *testing.T) {
	r := parseRetry(5)
	if r == nil || r.Max != 2 {
		t.Errorf("expected max=2 (clamped), got %v", r)
	}
}

func TestParseRetryMap(t *testing.T) {
	r := parseRetry(map[string]interface{}{"max": 1})
	if r == nil || r.Max != 1 {
		t.Errorf("expected max=1, got %v", r)
	}
}

func TestParseRetryNil(t *testing.T) {
	r := parseRetry(nil)
	if r != nil {
		t.Errorf("expected nil, got %v", r)
	}
}

func TestFormatTimeout(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{3600, "1h"},
		{5400, "1h 30m"},
		{90, "1m 30s"},
		{0, "1h"},
		{-1, "1h"},
		{61, "1m 1s"},
	}
	for _, tc := range tests {
		got := formatTimeout(tc.secs)
		if got != tc.want {
			t.Errorf("formatTimeout(%d) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

// Test timeout in pipeline YAML
func TestTimeoutInPipeline(t *testing.T) {
	yaml := `
test:
  timeout: 2m
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if job.Timeout != 120 {
		t.Errorf("expected timeout 120, got %d", job.Timeout)
	}
}

// Test retry in pipeline YAML
func TestRetryInPipeline(t *testing.T) {
	yaml := `
test:
  retry: 1
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if job.Retry == nil || job.Retry.Max != 1 {
		t.Errorf("expected retry max=1, got %v", job.Retry)
	}
}

// Test retry with map syntax in pipeline YAML
func TestRetryMapInPipeline(t *testing.T) {
	yaml := `
test:
  retry:
    max: 2
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if job.Retry == nil || job.Retry.Max != 2 {
		t.Errorf("expected retry max=2, got %v", job.Retry)
	}
}

// Test default timeout (no timeout specified)
func TestDefaultTimeoutInPipeline(t *testing.T) {
	yaml := `
test:
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	job := def.Jobs["test"]
	if job.Timeout != 0 {
		t.Errorf("expected timeout 0 (unset), got %d", job.Timeout)
	}
}
