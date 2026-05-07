package gcf

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodePodManifestRoundtrip(t *testing.T) {
	members := []PodMemberSpec{
		{Name: "postgres", BaseImageRef: "postgres:16", Cmd: []string{"postgres"}},
		{Name: "main", BaseImageRef: "alpine:latest", Cmd: []string{"echo", "hi"}, Workdir: "/work"},
	}
	enc, err := EncodePodManifest(members)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	var got []PodMemberJSON
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len: got %d want 2", len(got))
	}
	if got[0].Root != "/containers/postgres" {
		t.Errorf("postgres root: %q", got[0].Root)
	}
	if got[1].Root != "/containers/main" {
		t.Errorf("main root: %q", got[1].Root)
	}
	if got[1].Workdir != "/work" {
		t.Errorf("main workdir: %q", got[1].Workdir)
	}
}

func TestRenderPodOverlayDockerfile(t *testing.T) {
	spec := PodOverlaySpec{
		PodName:             "ci-pod",
		MainName:            "main",
		BootstrapBinaryPath: "/tmp/bootstrap",
		Members: []PodMemberSpec{
			{Name: "main", BaseImageRef: "us-central1-docker.pkg.dev/p/r/alpine:latest", Cmd: []string{"echo", "hi"}},
			{Name: "postgres", BaseImageRef: "us-central1-docker.pkg.dev/p/r/postgres:16", Cmd: []string{"postgres"}},
		},
	}
	df, err := RenderPodOverlayDockerfile(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(df, "FROM us-central1-docker.pkg.dev/p/r/alpine:latest") {
		t.Errorf("missing FROM <main>:\n%s", df)
	}
	if !strings.Contains(df, "cp -a /. /containers/main/") {
		t.Errorf("missing base-rootfs snapshot for main:\n%s", df)
	}
	if !strings.Contains(df, "COPY --from=us-central1-docker.pkg.dev/p/r/postgres:16 / /containers/postgres/") {
		t.Errorf("missing COPY --from for postgres:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_POD_CONTAINERS=") {
		t.Errorf("missing pod manifest env:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_POD_MAIN=main") {
		t.Errorf("missing main env:\n%s", df)
	}
	if !strings.Contains(df, `ENTRYPOINT ["/opt/sockerless/sockerless-gcf-bootstrap"]`) {
		t.Errorf("missing entrypoint:\n%s", df)
	}
}

func TestRenderPodOverlayDockerfileRejectsEmpty(t *testing.T) {
	if _, err := RenderPodOverlayDockerfile(PodOverlaySpec{BootstrapBinaryPath: "/x"}); err == nil {
		t.Fatal("expected error on empty Members")
	}
	if _, err := RenderPodOverlayDockerfile(PodOverlaySpec{
		Members: []PodMemberSpec{{Name: "x", BaseImageRef: "alpine"}},
	}); err == nil {
		t.Fatal("expected error on empty BootstrapBinaryPath")
	}
	if _, err := RenderPodOverlayDockerfile(PodOverlaySpec{
		BootstrapBinaryPath: "/x",
		Members:             []PodMemberSpec{{Name: "x"}},
	}); err == nil {
		t.Fatal("expected error on missing BaseImageRef")
	}
	if _, err := RenderPodOverlayDockerfile(PodOverlaySpec{
		BootstrapBinaryPath: "/x",
		Members:             []PodMemberSpec{{BaseImageRef: "alpine"}},
	}); err == nil {
		t.Fatal("expected error on missing Name")
	}
}

func TestPodOverlayContentTagStableAcrossOrder(t *testing.T) {
	a := PodOverlaySpec{
		BootstrapBinaryPath: "/tmp/bootstrap",
		MainName:            "main",
		Members: []PodMemberSpec{
			{Name: "postgres", BaseImageRef: "postgres:16"},
			{Name: "main", BaseImageRef: "alpine:latest", Cmd: []string{"echo", "hi"}},
		},
	}
	b := a
	if PodOverlayContentTag(a) != PodOverlayContentTag(b) {
		t.Error("expected identical specs to produce identical tags")
	}
	c := a
	c.Members = []PodMemberSpec{
		{Name: "main", BaseImageRef: "alpine:latest", Cmd: []string{"echo", "hi"}},
		{Name: "postgres", BaseImageRef: "postgres:16"},
	}
	if PodOverlayContentTag(a) == PodOverlayContentTag(c) {
		t.Error("expected member-order change to produce different tag")
	}
	d := a
	d.MainName = "other"
	if PodOverlayContentTag(a) == PodOverlayContentTag(d) {
		t.Error("expected MainName change to produce different tag")
	}
	if got := PodOverlayContentTag(a); !strings.HasPrefix(got, "gcf-pod-") || len(got) != len("gcf-pod-")+16 {
		t.Errorf("tag format: %q", got)
	}
}
