package lambda

import (
	"strings"
	"testing"
)

func TestRenderOverlayDockerfile_RequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		spec    OverlayImageSpec
		wantErr string
	}{
		{
			name:    "missing base",
			spec:    OverlayImageSpec{AgentBinaryPath: "a", BootstrapBinaryPath: "b"},
			wantErr: "BaseImageRef",
		},
		{
			name:    "missing agent",
			spec:    OverlayImageSpec{BaseImageRef: "alpine", BootstrapBinaryPath: "b"},
			wantErr: "AgentBinaryPath",
		},
		{
			name:    "missing bootstrap",
			spec:    OverlayImageSpec{BaseImageRef: "alpine", AgentBinaryPath: "a"},
			wantErr: "BootstrapBinaryPath",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RenderOverlayDockerfile(tc.spec); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRenderOverlayDockerfile_BasicStructure(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "123.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/alpine:latest",
		AgentBinaryPath:     "bin/sockerless-agent",
		BootstrapBinaryPath: "bin/sockerless-lambda-bootstrap",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"FROM 123.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/alpine:latest",
		"COPY bin/sockerless-agent /opt/sockerless/sockerless-agent",
		"COPY bin/sockerless-lambda-bootstrap /opt/sockerless/sockerless-lambda-bootstrap",
		`ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`,
	} {
		if !strings.Contains(df, want) {
			t.Errorf("rendered Dockerfile missing %q\n---\n%s", want, df)
		}
	}
	// No user env vars when entrypoint/cmd are empty.
	if strings.Contains(df, "SOCKERLESS_USER_ENTRYPOINT") {
		t.Errorf("did not expect user entrypoint env var, got:\n%s", df)
	}
}

func TestRenderOverlayDockerfile_UserEntrypointAndCmd(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		AgentBinaryPath:     "a",
		BootstrapBinaryPath: "b",
		UserEntrypoint:      []string{"/bin/sh", "-c"},
		UserCmd:             []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_USER_ENTRYPOINT=/bin/sh:-c") {
		t.Errorf("entrypoint env var missing or malformed:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_USER_CMD=echo:hello") {
		t.Errorf("cmd env var missing or malformed:\n%s", df)
	}
}
