package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMaterialiseBindLinks(t *testing.T) {
	root := t.TempDir()
	mnt := filepath.Join(root, "mnt", "sockerless-shared")
	if err := os.MkdirAll(filepath.Join(mnt, "_work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mnt, "externals"), 0o755); err != nil {
		t.Fatal(err)
	}
	dstWork := filepath.Join(root, "__w")
	dstExt := filepath.Join(root, "__e")
	spec := dstWork + "=" + filepath.Join(mnt, "_work") + "," + dstExt + "=" + filepath.Join(mnt, "externals")
	if err := materialiseBindLinks(spec); err != nil {
		t.Fatalf("first call: %v", err)
	}
	for _, d := range []string{dstWork, dstExt} {
		fi, err := os.Lstat(d)
		if err != nil {
			t.Fatalf("lstat %s: %v", d, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s not a symlink", d)
		}
	}
	// Idempotent: rerunning leaves the same symlinks intact.
	if err := materialiseBindLinks(spec); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestMaterialiseBindLinks_RejectsRelativeDst(t *testing.T) {
	if err := materialiseBindLinks("__w=/mnt/sockerless-shared/_work"); err == nil {
		t.Fatal("expected error for relative dst")
	}
}

func TestMaterialiseBindLinks_EmptyAccepted(t *testing.T) {
	if err := materialiseBindLinks(""); err != nil {
		t.Fatalf("empty spec should be accepted: %v", err)
	}
}

// encodeArgv matches the backend's encoding: base64(JSON(argv)).
func encodeArgv(argv []string) string {
	b, _ := json.Marshal(argv)
	return base64.StdEncoding.EncodeToString(b)
}

// TestHandleOneInvocation_RoundTrip verifies the Runtime-API loop
// (single iteration) polls /next, runs the user entrypoint with the
// payload on stdin, and posts /response with the stdout.
func TestHandleOneInvocation_RoundTrip(t *testing.T) {
	var posted atomic.Value
	var gotBody atomic.Value
	var nextCount int32

	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/invocation/next", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&nextCount, 1)
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", "req-1")
		w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", "arn:aws:lambda:us-east-1:000000000000:function:test")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"echo":"hello"}`))
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-1/response", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted.Store("response")
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-1/error", func(w http.ResponseWriter, r *http.Request) {
		posted.Store("error")
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Configure a user entrypoint that echoes stdin to stdout.
	t.Setenv(envUserEntrypoint, encodeArgv([]string{"/bin/cat"}))
	t.Setenv(envUserCmd, "")

	if err := handleOneInvocation(srv.URL); err != nil {
		t.Fatalf("handleOneInvocation: %v", err)
	}

	if nextCount != 1 {
		t.Errorf("/next should be called once, got %d", nextCount)
	}
	if p := posted.Load(); p != "response" {
		t.Errorf("want /response, got %v", p)
	}
	if b := gotBody.Load(); b == nil || !strings.Contains(b.(string), `"echo":"hello"`) {
		t.Errorf("want echoed payload, got %v", b)
	}
}

// TestHandleOneInvocation_UserError verifies that a non-zero user exit
// code posts to /error with an errorMessage envelope.
func TestHandleOneInvocation_UserError(t *testing.T) {
	var posted atomic.Value
	var gotBody atomic.Value

	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/invocation/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", "req-err")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-err/response", func(w http.ResponseWriter, r *http.Request) {
		posted.Store("response")
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/2018-06-01/runtime/invocation/req-err/error", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		posted.Store("error")
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv(envUserEntrypoint, encodeArgv([]string{"/bin/sh"}))
	t.Setenv(envUserCmd, encodeArgv([]string{"-c", "exit 7"}))

	if err := handleOneInvocation(srv.URL); err != nil {
		t.Fatalf("handleOneInvocation: %v", err)
	}

	if p := posted.Load(); p != "error" {
		t.Errorf("want /error, got %v", p)
	}
	b, _ := gotBody.Load().(string)
	if !strings.Contains(b, "errorMessage") {
		t.Errorf("want errorMessage in body, got %q", b)
	}
}

// TestRunUserInvocation_NoEntrypoint verifies the "echo payload"
// fallback when no user entrypoint is configured (matches the
// testdata handler's semantics).
func TestRunUserInvocation_NoEntrypoint(t *testing.T) {
	t.Setenv(envUserEntrypoint, "")
	t.Setenv(envUserCmd, "")

	stdout, _, exit := runUserInvocation(context.Background(), []byte(`{"k":"v"}`))
	if exit != 0 {
		t.Errorf("want exit 0, got %d", exit)
	}
	if !bytes.Equal(stdout, []byte(`{"k":"v"}`)) {
		t.Errorf("want payload echoed, got %q", string(stdout))
	}
}

// TestBuildErrorPayload verifies the error envelope shape.
func TestBuildErrorPayload(t *testing.T) {
	body := buildErrorPayload([]byte("boom\n"), 5)
	s := string(body)
	if !strings.Contains(s, `"errorMessage":"boom"`) {
		t.Errorf("missing stderr message: %s", s)
	}
	if !strings.Contains(s, `"errorType":"HandlerError"`) {
		t.Errorf("missing errorType: %s", s)
	}
}

// TestBuildErrorPayload_EmptyStderr verifies the fallback message when
// the user process exits non-zero without writing to stderr.
func TestBuildErrorPayload_EmptyStderr(t *testing.T) {
	body := buildErrorPayload(nil, 3)
	s := string(body)
	if !strings.Contains(s, `user process exited 3`) {
		t.Errorf("want fallback message, got %s", s)
	}
}

// TestPostInitError verifies the init-error envelope hits the right
// Runtime-API endpoint.
func TestPostInitError(t *testing.T) {
	var called atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/2018-06-01/runtime/init/error", func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"errorType":"InitError"`) {
			t.Errorf("want InitError type, got %s", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	postInitError(srv.URL, "dial failed: boom")

	if called.Load() != 1 {
		t.Errorf("init/error should be hit once, got %d", called.Load())
	}
}

// — Pod supervisor tests (mirror of gcf bootstrap tests) —

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
	if len(got) != 2 {
		t.Fatalf("len: got %d want 2", len(got))
	}
	if got[0].Name != "web" || got[1].Cmd[0] != "postgres" {
		t.Errorf("decoded: %+v", got)
	}
}

func TestPickPodMainDefaultsToLast(t *testing.T) {
	pod := []PodMember{{Name: "postgres"}, {Name: "redis"}, {Name: "main"}}
	t.Setenv(envPodMain, "")
	got, ok := pickPodMain(pod)
	if !ok || got.Name != "main" {
		t.Fatalf("expected last entry, got %+v", got)
	}
}

func TestPickPodMainHonoursEnv(t *testing.T) {
	pod := []PodMember{{Name: "postgres"}, {Name: "redis"}, {Name: "main"}}
	t.Setenv(envPodMain, "redis")
	got, ok := pickPodMain(pod)
	if !ok || got.Name != "redis" {
		t.Fatalf("expected redis, got %+v", got)
	}
}

func TestBuildPodMemberCmdRequiresArgv(t *testing.T) {
	if _, err := buildPodMemberCmd(PodMember{Name: "x", Root: "/r"}, nil); err == nil {
		t.Fatal("expected error when no entrypoint or cmd")
	}
}

func TestBuildPodMemberCmdRequiresRoot(t *testing.T) {
	if _, err := buildPodMemberCmd(PodMember{Name: "x", Cmd: []string{"echo", "hi"}}, nil); err == nil {
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
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasSuffix(cmd.Path, "/sh") {
		t.Errorf("expected /bin/sh, got %q", cmd.Path)
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
