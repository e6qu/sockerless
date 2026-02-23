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

func TestParseDockerfileHealthcheckShell(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nHEALTHCHECK CMD curl -f http://localhost/", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.Healthcheck == nil {
		t.Fatal("expected Healthcheck to be set")
	}
	if len(p.config.Healthcheck.Test) != 2 || p.config.Healthcheck.Test[0] != "CMD-SHELL" {
		t.Errorf("test = %v, want [CMD-SHELL curl -f http://localhost/]", p.config.Healthcheck.Test)
	}
}

func TestParseDockerfileHealthcheckExec(t *testing.T) {
	p, err := parseDockerfile(`FROM alpine
HEALTHCHECK CMD ["curl", "-f", "http://localhost/"]`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.Healthcheck == nil {
		t.Fatal("expected Healthcheck to be set")
	}
	if len(p.config.Healthcheck.Test) != 4 || p.config.Healthcheck.Test[0] != "CMD" {
		t.Errorf("test = %v, want [CMD curl -f http://localhost/]", p.config.Healthcheck.Test)
	}
}

func TestParseDockerfileHealthcheckNone(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nHEALTHCHECK NONE", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.Healthcheck == nil {
		t.Fatal("expected Healthcheck to be set")
	}
	if len(p.config.Healthcheck.Test) != 1 || p.config.Healthcheck.Test[0] != "NONE" {
		t.Errorf("test = %v, want [NONE]", p.config.Healthcheck.Test)
	}
}

func TestParseDockerfileHealthcheckOptions(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nHEALTHCHECK --interval=5s --timeout=3s --retries=3 --start-period=10s CMD curl -f http://localhost/", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hc := p.config.Healthcheck
	if hc == nil {
		t.Fatal("expected Healthcheck to be set")
	}
	if hc.Interval != 5000000000 { // 5s in nanoseconds
		t.Errorf("interval = %d, want 5000000000", hc.Interval)
	}
	if hc.Timeout != 3000000000 {
		t.Errorf("timeout = %d, want 3000000000", hc.Timeout)
	}
	if hc.Retries != 3 {
		t.Errorf("retries = %d, want 3", hc.Retries)
	}
	if hc.StartPeriod != 10000000000 {
		t.Errorf("start period = %d, want 10000000000", hc.StartPeriod)
	}
}

func TestParseDockerfileHealthcheckMultiStageReset(t *testing.T) {
	dockerfile := `FROM node:16
HEALTHCHECK CMD curl -f http://localhost/
FROM alpine
CMD ["echo"]`
	p, err := parseDockerfile(dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// HEALTHCHECK from first stage should be reset
	if p.config.Healthcheck != nil {
		t.Errorf("expected nil Healthcheck after multi-stage reset, got %v", p.config.Healthcheck)
	}
}

func TestParseDockerfileBuildPreservesHealthcheck(t *testing.T) {
	dockerfile := `FROM alpine
HEALTHCHECK --interval=30s CMD wget -qO- http://localhost/ || exit 1`
	p, err := parseDockerfile(dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.Healthcheck == nil {
		t.Fatal("expected Healthcheck to be preserved")
	}
	if p.config.Healthcheck.Test[0] != "CMD-SHELL" {
		t.Errorf("test[0] = %q, want CMD-SHELL", p.config.Healthcheck.Test[0])
	}
	if p.config.Healthcheck.Interval != 30000000000 {
		t.Errorf("interval = %d, want 30000000000", p.config.Healthcheck.Interval)
	}
}

func TestParseDockerfileShell(t *testing.T) {
	p, err := parseDockerfile(`FROM alpine
SHELL ["/bin/bash", "-c"]`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.config.Shell) != 2 || p.config.Shell[0] != "/bin/bash" || p.config.Shell[1] != "-c" {
		t.Errorf("shell = %v, want [/bin/bash -c]", p.config.Shell)
	}
}

func TestParseDockerfileShellMultiStageReset(t *testing.T) {
	dockerfile := `FROM node:16
SHELL ["/bin/bash", "-c"]
FROM alpine
CMD ["echo"]`
	p, err := parseDockerfile(dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.config.Shell) != 0 {
		t.Errorf("expected nil Shell after multi-stage reset, got %v", p.config.Shell)
	}
}

func TestParseDockerfileStopSignal(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nSTOPSIGNAL SIGTERM", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.StopSignal != "SIGTERM" {
		t.Errorf("stop signal = %q, want SIGTERM", p.config.StopSignal)
	}
}

func TestParseDockerfileVolumeJSON(t *testing.T) {
	p, err := parseDockerfile(`FROM alpine
VOLUME ["/data", "/logs"]`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.config.Volumes["/data"]; !ok {
		t.Errorf("volumes = %v, want /data", p.config.Volumes)
	}
	if _, ok := p.config.Volumes["/logs"]; !ok {
		t.Errorf("volumes = %v, want /logs", p.config.Volumes)
	}
}

func TestParseDockerfileVolumeSpaceSeparated(t *testing.T) {
	p, err := parseDockerfile("FROM alpine\nVOLUME /data /logs", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.config.Volumes["/data"]; !ok {
		t.Errorf("volumes = %v, want /data", p.config.Volumes)
	}
	if _, ok := p.config.Volumes["/logs"]; !ok {
		t.Errorf("volumes = %v, want /logs", p.config.Volumes)
	}
}
