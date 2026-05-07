package gcpcommon

import (
	"strings"
	"testing"
)

// TestOverlayContentTag_IndependentOfEntrypointCmdWorkdir asserts the
// core invariant: the content-hash MUST stay stable when only
// entrypoint/cmd/workdir differ. Otherwise pool entries can't be
// reused across containers with different commands — gitlab-runner
// sets distinct entrypoint/cmd per container type, which would
// fragment the pool.
//
// The runtime-env-override path (SOCKERLESS_USER_ENTRYPOINT et al,
// applied per claim by the backend) is what makes this safe — the
// same overlay image executes any user command at request time.
func TestOverlayContentTag_IndependentOfEntrypointCmdWorkdir(t *testing.T) {
	base := OverlayImageSpec{
		BaseImageRef:        "registry.gitlab.com/foo/runner-helper:v17",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-gcf-bootstrap",
	}
	want := OverlayContentTag("gcf-", base)

	cases := []OverlayImageSpec{
		base,
		{
			BaseImageRef:        base.BaseImageRef,
			BootstrapBinaryPath: base.BootstrapBinaryPath,
			UserEntrypoint:      []string{"/usr/bin/dumb-init", "/entrypoint"},
			UserCmd:             []string{"helper", "cache-init", "/cache"},
		},
		{
			BaseImageRef:        base.BaseImageRef,
			BootstrapBinaryPath: base.BootstrapBinaryPath,
			UserEntrypoint:      []string{"/usr/bin/dumb-init", "/entrypoint"},
			UserCmd:             []string{"helper", "set-permission", "/perm"},
		},
		{
			BaseImageRef:        base.BaseImageRef,
			BootstrapBinaryPath: base.BootstrapBinaryPath,
			UserWorkdir:         "/builds/repo",
		},
	}
	for i, spec := range cases {
		got := OverlayContentTag("gcf-", spec)
		if got != want {
			t.Errorf("case %d: contentTag = %q, want %q (must be stable across entrypoint/cmd/workdir)", i, got, want)
		}
	}
}

// TestOverlayContentTag_DiffersOnImageOrBootstrap — content-hash MUST
// still differ when (image, bootstrap) changes; otherwise pool reuse
// would route to the wrong overlay.
func TestOverlayContentTag_DiffersOnImageOrBootstrap(t *testing.T) {
	a := OverlayContentTag("gcf-", OverlayImageSpec{
		BaseImageRef: "image:a", BootstrapBinaryPath: "/bs",
	})
	b := OverlayContentTag("gcf-", OverlayImageSpec{
		BaseImageRef: "image:b", BootstrapBinaryPath: "/bs",
	})
	c := OverlayContentTag("gcf-", OverlayImageSpec{
		BaseImageRef: "image:a", BootstrapBinaryPath: "/other",
	})
	if a == b {
		t.Error("different images must produce different contentTag")
	}
	if a == c {
		t.Error("different bootstrap paths must produce different contentTag")
	}
}

// TestRenderOverlayDockerfile_NoUserEnvBaked — the rendered Dockerfile
// must NOT bake SOCKERLESS_USER_* env vars. Those are passed
// at runtime via ServiceConfig.EnvironmentVariables on each fresh
// deploy + each pool claim.
func TestRenderOverlayDockerfile_NoUserEnvBaked(t *testing.T) {
	df, err := RenderOverlayDockerfile(OverlayImageSpec{
		BaseImageRef:        "alpine:latest",
		BootstrapBinaryPath: "/opt/sockerless/sockerless-gcf-bootstrap",
		UserEntrypoint:      []string{"/should-not-bake"},
		UserCmd:             []string{"--also-no"},
		UserWorkdir:         "/no",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"SOCKERLESS_USER_ENTRYPOINT", "SOCKERLESS_USER_CMD", "SOCKERLESS_USER_WORKDIR"} {
		if strings.Contains(df, banned) {
			t.Errorf("Dockerfile must not bake %s — runtime env injection is required. Got:\n%s", banned, df)
		}
	}
	if !strings.Contains(df, "ENTRYPOINT [\"/opt/sockerless/sockerless-gcf-bootstrap\"]") {
		t.Errorf("Dockerfile must still set the bootstrap as ENTRYPOINT. Got:\n%s", df)
	}
}
