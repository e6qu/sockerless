package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParsePodManifestEmpty(t *testing.T) {
	t.Setenv(envPodContainers, "")
	if _, ok := parsePodManifest(); ok {
		t.Fatal("expected unset env to return ok=false")
	}
}

func TestParsePodManifestRoundtrip(t *testing.T) {
	want := []PodMember{
		{Name: "web", Root: "/containers/web", Entrypoint: []string{"nginx", "-g", "daemon off;"}},
		{Name: "db", Root: "/containers/db", Cmd: []string{"postgres"}, Env: []string{"POSTGRES_PASSWORD=x"}},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Setenv(envPodContainers, base64.StdEncoding.EncodeToString(raw))
	got, ok := parsePodManifest()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Root != want[i].Root {
			t.Errorf("entry %d mismatch: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestPickPodMainDefaultsToLast(t *testing.T) {
	pod := []PodMember{
		{Name: "postgres"},
		{Name: "redis"},
		{Name: "main"},
	}
	t.Setenv(envPodMain, "")
	got, ok := pickPodMain(pod)
	if !ok || got.Name != "main" {
		t.Fatalf("expected last entry as main, got %+v", got)
	}
}

func TestPickPodMainHonoursEnv(t *testing.T) {
	pod := []PodMember{
		{Name: "postgres"},
		{Name: "redis"},
		{Name: "main"},
	}
	t.Setenv(envPodMain, "redis")
	got, ok := pickPodMain(pod)
	if !ok || got.Name != "redis" {
		t.Fatalf("expected redis, got %+v", got)
	}
}

func TestPickPodMainEmpty(t *testing.T) {
	if _, ok := pickPodMain(nil); ok {
		t.Fatal("expected ok=false on nil pod")
	}
}

func TestBuildPodMemberCmdRequiresArgv(t *testing.T) {
	if _, err := buildPodMemberCmd(PodMember{Name: "x", Root: "/r"}); err == nil {
		t.Fatal("expected error when no entrypoint or cmd")
	}
}

func TestBuildPodMemberCmdRequiresRoot(t *testing.T) {
	if _, err := buildPodMemberCmd(PodMember{Name: "x", Cmd: []string{"echo", "hi"}}); err == nil {
		t.Fatal("expected error when no chroot root")
	}
}

func TestBuildPodMemberCmdShellWrap(t *testing.T) {
	cmd, err := buildPodMemberCmd(PodMember{
		Name:       "x",
		Root:       "/containers/x",
		Entrypoint: []string{"/usr/bin/printf"},
		Cmd:        []string{"%s", "hi there"},
		Workdir:    "/work",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := cmd.Path; !strings.HasSuffix(got, "/sh") {
		t.Errorf("expected /bin/sh, got %q", got)
	}
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.Chroot != "/containers/x" {
		t.Errorf("expected Chroot=/containers/x, got %+v", cmd.SysProcAttr)
	}
	if cmd.Dir != "/work" {
		t.Errorf("expected Dir=/work, got %q", cmd.Dir)
	}
	want := `'/usr/bin/printf' '%s' 'hi there'`
	if cmd.Args[2] != want {
		t.Errorf("shell line:\n got: %s\nwant: %s", cmd.Args[2], want)
	}
}

func TestBuildPodMemberCmdQuotesEmbeddedSingle(t *testing.T) {
	cmd, err := buildPodMemberCmd(PodMember{
		Name: "x", Root: "/r",
		Cmd: []string{"echo", "it's"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := `'echo' 'it'\''s'`
	if cmd.Args[2] != want {
		t.Errorf("got %q want %q", cmd.Args[2], want)
	}
}

func TestPrefixWriterWrapsLines(t *testing.T) {
	var buf bytes.Buffer
	w := newPrefixWriter(&buf, "[svc] ")
	if _, err := w.Write([]byte("line1\nline2\n")); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if _, err := w.Write([]byte("part-a")); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if _, err := w.Write([]byte("part-b\n")); err != nil {
		t.Fatalf("write3: %v", err)
	}
	got := buf.String()
	want := "[svc] line1\n[svc] line2\n[svc] part-apart-b\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestParseUserArgvDecodes(t *testing.T) {
	raw, _ := json.Marshal([]string{"echo", "hello"})
	t.Setenv("SOCKERLESS_USER_CMD", base64.StdEncoding.EncodeToString(raw))
	got := parseUserArgv("SOCKERLESS_USER_CMD")
	if len(got) != 2 || got[0] != "echo" || got[1] != "hello" {
		t.Errorf("got %v want [echo hello]", got)
	}
}
