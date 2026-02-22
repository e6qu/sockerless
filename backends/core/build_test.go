package core

import (
	"strings"
	"testing"
)

func TestParseDockerfileBasic(t *testing.T) {
	dockerfile := `FROM node:16-buster-slim
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
`
	p, err := parseDockerfile(dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.from != "node:16-buster-slim" {
		t.Errorf("from = %q, want node:16-buster-slim", p.from)
	}
	if len(p.copies) != 1 {
		t.Fatalf("copies = %d, want 1", len(p.copies))
	}
	if p.copies[0].src != "entrypoint.sh" || p.copies[0].dst != "/entrypoint.sh" {
		t.Errorf("copy = %+v, want src=entrypoint.sh dst=/entrypoint.sh", p.copies[0])
	}
	if len(p.config.Entrypoint) != 1 || p.config.Entrypoint[0] != "/entrypoint.sh" {
		t.Errorf("entrypoint = %v, want [\"/entrypoint.sh\"]", p.config.Entrypoint)
	}
}

func TestParseDockerfileEnv(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nENV FOO=bar", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range p.config.Env {
		if e == "FOO=bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("env = %v, want FOO=bar", p.config.Env)
	}
}

func TestParseDockerfileEnvSpaceForm(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nENV FOO bar baz", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range p.config.Env {
		if e == "FOO=bar baz" {
			found = true
		}
	}
	if !found {
		t.Errorf("env = %v, want FOO=bar baz", p.config.Env)
	}
}

func TestParseDockerfileCmd(t *testing.T) {
	// JSON form
	p, err := parseDockerfile(`FROM alpine
CMD ["sh", "-c", "echo"]`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.config.Cmd) != 3 || p.config.Cmd[0] != "sh" || p.config.Cmd[1] != "-c" || p.config.Cmd[2] != "echo" {
		t.Errorf("cmd = %v, want [sh -c echo]", p.config.Cmd)
	}

	// Shell form
	p, err = parseDockerfile("FROM alpine\nCMD echo hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.config.Cmd) != 2 || p.config.Cmd[0] != "echo" || p.config.Cmd[1] != "hello" {
		t.Errorf("cmd = %v, want [echo hello]", p.config.Cmd)
	}
}

func TestParseDockerfileWorkdir(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nWORKDIR /app", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.WorkingDir != "/app" {
		t.Errorf("workdir = %q, want /app", p.config.WorkingDir)
	}
}

func TestParseDockerfileLabel(t *testing.T) {
	p, err := parseDockerfile(`FROM alpine
LABEL version="1.0"`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := p.config.Labels["version"]; !ok || v != "1.0" {
		t.Errorf("labels = %v, want version=1.0", p.config.Labels)
	}
}

func TestParseDockerfileExpose(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nEXPOSE 8080/tcp", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.config.ExposedPorts["8080/tcp"]; !ok {
		t.Errorf("exposed ports = %v, want 8080/tcp", p.config.ExposedPorts)
	}
}

func TestParseDockerfileExposeDefault(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nEXPOSE 3000", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.config.ExposedPorts["3000/tcp"]; !ok {
		t.Errorf("exposed ports = %v, want 3000/tcp", p.config.ExposedPorts)
	}
}

func TestParseDockerfileUser(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nUSER nobody", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.User != "nobody" {
		t.Errorf("user = %q, want nobody", p.config.User)
	}
}

func TestParseDockerfileArg(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nARG VERSION=1.0\nLABEL v=$VERSION", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := p.config.Labels["v"]; !ok || v != "1.0" {
		t.Errorf("labels = %v, want v=1.0", p.config.Labels)
	}
}

func TestParseDockerfileBuildArg(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nARG VERSION=1.0\nLABEL v=$VERSION",
		map[string]string{"VERSION": "2.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := p.config.Labels["v"]; !ok || v != "2.0" {
		t.Errorf("labels = %v, want v=2.0", p.config.Labels)
	}
}

func TestParseDockerfileLineContinuation(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nRUN echo \\\n  hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.from != "alpine" {
		t.Errorf("from = %q, want alpine", p.from)
	}
}

func TestParseDockerfileMultiStage(t *testing.T) {
	dockerfile := `FROM golang:1.21 AS builder
WORKDIR /build
COPY . .
RUN go build

FROM alpine:3.18
COPY --from=builder /build/app /app
ENTRYPOINT ["/app"]
`
	p, err := parseDockerfile(dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.from != "alpine:3.18" {
		t.Errorf("from = %q, want alpine:3.18 (final stage)", p.from)
	}
	// COPY --from=builder should be skipped
	if len(p.copies) != 0 {
		t.Errorf("copies = %d, want 0 (--from copies skipped)", len(p.copies))
	}
	// WorkingDir should be reset (not /build from first stage)
	if p.config.WorkingDir != "" {
		t.Errorf("workdir = %q, want empty (reset after second FROM)", p.config.WorkingDir)
	}
	if len(p.config.Entrypoint) != 1 || p.config.Entrypoint[0] != "/app" {
		t.Errorf("entrypoint = %v, want [\"/app\"]", p.config.Entrypoint)
	}
}

func TestParseDockerfileCopyFromStage(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nCOPY --from=builder /app /app", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.copies) != 0 {
		t.Errorf("copies = %d, want 0 (--from copies should be skipped)", len(p.copies))
	}
}

func TestParseDockerfileNoFrom(t *testing.T) {
	_, err := parseDockerfile("RUN echo hello", nil)
	if err == nil {
		t.Error("expected error for Dockerfile without FROM")
	}
	if !strings.Contains(err.Error(), "no FROM") {
		t.Errorf("error = %q, want 'no FROM'", err.Error())
	}
}
