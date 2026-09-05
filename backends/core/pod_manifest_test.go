package core

import (
	"reflect"
	"testing"
)

func TestPodManifestRoundTrip(t *testing.T) {
	members := []PodMemberSpec{
		{Name: "job", ContainerID: "c1", BaseImageRef: "alpine:3", Entrypoint: []string{"sh"}, Cmd: []string{"-c", "sleep"}, Workdir: "/w", Env: []string{"A=1"}},
		{Name: "postgres", ContainerID: "c2", BaseImageRef: "postgres:16"},
	}
	enc, err := EncodePodManifest(members)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePodManifest(enc)
	if err != nil {
		t.Fatal(err)
	}
	want := []PodMemberJSON{
		{Name: "job", Root: "/containers/job", Entrypoint: []string{"sh"}, Cmd: []string{"-c", "sleep"}, Env: []string{"A=1"}, Workdir: "/w", ContainerID: "c1", Image: "alpine:3"},
		{Name: "postgres", Root: "/containers/postgres", ContainerID: "c2", Image: "postgres:16"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip = %+v, want %+v", got, want)
	}
	if empty, err := DecodePodManifest(""); err != nil || empty != nil {
		t.Fatalf("empty manifest = %v, %v; want nil, nil", empty, err)
	}
	if _, err := DecodePodManifest("!!not-base64"); err == nil {
		t.Fatal("malformed base64 must error")
	}
	if _, err := DecodePodManifest("bm90IGpzb24="); err == nil {
		t.Fatal("non-JSON payload must error")
	}
}

func TestSanitizePodNames(t *testing.T) {
	for in, want := range map[string]string{"Job_Runner.v2": "job-runner-v2", "ok-name": "ok-name", "!!!": "x", "": "x"} {
		if got := SanitizePodMemberName(in); got != want {
			t.Errorf("SanitizePodMemberName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Job_Runner.v2": "job_runnerv2", "ok-name": "ok-name", "a b": "ab"} {
		if got := SanitizePodLabelValue(in); got != want {
			t.Errorf("SanitizePodLabelValue(%q) = %q, want %q", in, got, want)
		}
	}
}
