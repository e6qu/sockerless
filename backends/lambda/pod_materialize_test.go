package lambda

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestContainersToPodOverlaySpecLambda(t *testing.T) {
	containers := []api.Container{
		{
			ID:   "111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Name: "/postgres",
			Config: api.ContainerConfig{
				Image: "729079515331.dkr.ecr.eu-west-1.amazonaws.com/postgres:16",
				Cmd:   []string{"postgres"},
				Env:   []string{"POSTGRES_PASSWORD=x"},
			},
		},
		{
			ID:   "222222222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Name: "/main-step",
			Config: api.ContainerConfig{
				Image:      "729079515331.dkr.ecr.eu-west-1.amazonaws.com/alpine:latest",
				Entrypoint: []string{"sh", "-c"},
				Cmd:        []string{"echo hello && pg_isready -h localhost"},
				WorkingDir: "/work",
			},
		},
	}
	mainID := containers[1].ID
	spec := containersToPodOverlaySpec("/tmp/agent", "/tmp/bootstrap", "ci-pod", mainID, containers)

	if spec.PodName != "ci-pod" {
		t.Errorf("PodName: %q", spec.PodName)
	}
	if spec.MainName != "main-step" {
		t.Errorf("MainName: %q want main-step", spec.MainName)
	}
	if spec.AgentBinaryPath != "/tmp/agent" {
		t.Errorf("AgentBinaryPath: %q", spec.AgentBinaryPath)
	}
	if spec.BootstrapBinaryPath != "/tmp/bootstrap" {
		t.Errorf("BootstrapBinaryPath: %q", spec.BootstrapBinaryPath)
	}
	if len(spec.Members) != 2 {
		t.Fatalf("Members len: %d", len(spec.Members))
	}
	if spec.Members[0].Name != "postgres" || spec.Members[0].ContainerID != containers[0].ID {
		t.Errorf("members[0]: %+v", spec.Members[0])
	}
	if spec.Members[1].Name != "main-step" {
		t.Errorf("members[1].Name: %q", spec.Members[1].Name)
	}
}

func TestSanitizePodMemberNameLambda(t *testing.T) {
	cases := map[string]string{
		"":            "x",
		"OK-Name":     "ok-name",
		"main_step":   "main-step",
		"my.svc":      "my-svc",
		"weird!@#$":   "weird",
		"!!!@@@":      "x",
		"abc.123_xyz": "abc-123-xyz",
	}
	for in, want := range cases {
		if got := sanitizePodMemberName(in); got != want {
			t.Errorf("sanitizePodMemberName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSanitizePodTagValueLambda(t *testing.T) {
	cases := map[string]string{
		"OK-pod_1": "ok-pod_1",
		"my-pod":   "my-pod",
		"weird!":   "weird",
		"":         "",
		"UPPER":    "upper",
	}
	for in, want := range cases {
		if got := sanitizePodTagValue(in); got != want {
			t.Errorf("sanitizePodTagValue(%q) = %q want %q", in, got, want)
		}
	}
}

func TestRenderPodOverlayDockerfileLambda(t *testing.T) {
	spec := PodOverlaySpec{
		PodName:             "ci-pod",
		MainName:            "main",
		AgentBinaryPath:     "/tmp/agent",
		BootstrapBinaryPath: "/tmp/bootstrap",
		Members: []PodMemberSpec{
			{Name: "main", BaseImageRef: "729079515331.dkr.ecr.eu-west-1.amazonaws.com/alpine:latest", Cmd: []string{"echo", "hi"}},
			{Name: "postgres", BaseImageRef: "729079515331.dkr.ecr.eu-west-1.amazonaws.com/postgres:16", Cmd: []string{"postgres"}},
		},
	}
	df, err := RenderPodOverlayDockerfile(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(df, "FROM 729079515331.dkr.ecr.eu-west-1.amazonaws.com/alpine:latest") {
		t.Errorf("missing FROM <main>:\n%s", df)
	}
	if !strings.Contains(df, "cp -a /. /containers/main/") {
		t.Errorf("missing base-rootfs snapshot for main:\n%s", df)
	}
	if !strings.Contains(df, "COPY --from=729079515331.dkr.ecr.eu-west-1.amazonaws.com/postgres:16 / /containers/postgres/") {
		t.Errorf("missing COPY --from for postgres:\n%s", df)
	}
	if !strings.Contains(df, "COPY sockerless-agent /opt/sockerless/sockerless-agent") {
		t.Errorf("missing agent COPY:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_POD_CONTAINERS=") {
		t.Errorf("missing pod manifest env:\n%s", df)
	}
	if !strings.Contains(df, "ENV SOCKERLESS_POD_MAIN=main") {
		t.Errorf("missing main env:\n%s", df)
	}
	if !strings.Contains(df, `ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`) {
		t.Errorf("missing entrypoint:\n%s", df)
	}
}

func TestPodOverlayContentTagLambda(t *testing.T) {
	a := PodOverlaySpec{
		AgentBinaryPath:     "/tmp/agent",
		BootstrapBinaryPath: "/tmp/bootstrap",
		MainName:            "main",
		Members: []PodMemberSpec{
			{Name: "main", BaseImageRef: "alpine:latest", Cmd: []string{"echo", "hi"}},
			{Name: "postgres", BaseImageRef: "postgres:16"},
		},
	}
	tagA := PodOverlayContentTag(a)
	if !strings.HasPrefix(tagA, "overlay-pod-") || len(tagA) != len("overlay-pod-")+16 {
		t.Errorf("tag format: %q", tagA)
	}
	b := a
	if PodOverlayContentTag(b) != tagA {
		t.Error("expected identical specs to produce identical tags")
	}
	c := a
	c.MainName = "other"
	if PodOverlayContentTag(c) == tagA {
		t.Error("expected MainName change to produce different tag")
	}
}

func TestEncodeDecodePodManifestLambda(t *testing.T) {
	members := []PodMemberSpec{
		{Name: "postgres", ContainerID: "11111", BaseImageRef: "postgres:16", Cmd: []string{"postgres"}},
		{Name: "main", ContainerID: "22222", BaseImageRef: "alpine", Entrypoint: []string{"sh", "-c"}, Cmd: []string{"echo hi"}},
	}
	enc, err := EncodePodManifest(members)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodePodManifest(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len: %d", len(got))
	}
	if got[0].Root != "/containers/postgres" || got[0].ContainerID != "11111" {
		t.Errorf("members[0]: %+v", got[0])
	}
	if got[1].Image != "alpine" {
		t.Errorf("members[1].Image: %q", got[1].Image)
	}
}
