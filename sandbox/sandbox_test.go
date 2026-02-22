package sandbox

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestEcho(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunCommand(ctx, []string{"echo", "hello world"}, nil, dir, nil, nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestLs(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunCommand(ctx, []string{"ls", "/"}, nil, dir, nil, nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{"bin", "etc", "tmp", "usr", "var"} {
		if !strings.Contains(out, expected) {
			t.Errorf("ls / output missing %q: %s", expected, out)
		}
	}
}

func TestShellPipe(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunShell(ctx, "echo hello | tr a-z A-Z", nil, dir, nil, nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "HELLO" {
		t.Fatalf("got %q, want %q", got, "HELLO")
	}
}

func TestShellAndOr(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "true && echo yes || echo no", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "yes" {
		t.Fatalf("got %q, want %q", got, "yes")
	}
}

func TestShellCFlag(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, _ := rt.RunSimpleCommand(ctx, []string{"sh", "-c", "echo hello | tr a-z A-Z"}, nil, dir, nil, "", nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "HELLO" {
		t.Fatalf("got %q, want %q", got, "HELLO")
	}
}

func TestShellScriptFile(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Write a script file to the container's filesystem
	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.WriteFile(dir+"/var/run/act/workflow/0.sh", []byte("echo 'Hello from script'\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/var/run/act/workflow/0.sh"}, nil, dir, nil, "", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "Hello from script" {
		t.Fatalf("got %q, want %q", got, "Hello from script")
	}
}

func TestShellScriptEFlag(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Write a script that would fail on the first command with -e
	os.WriteFile(dir+"/tmp/fail.sh", []byte("false\necho should-not-reach\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, _ := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/tmp/fail.sh"}, nil, dir, nil, "", nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code with set -e and failing command")
	}
	if strings.Contains(stdout.String(), "should-not-reach") {
		t.Fatal("script continued after failing command despite -e flag")
	}
}

func TestShellECFlag(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, _ := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "-c", "echo hello from ec"}, nil, dir, nil, "", nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello from ec" {
		t.Fatalf("got %q, want %q", got, "hello from ec")
	}
}

func TestShellScriptWithWorkDir(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.MkdirAll(dir+"/app/src", 0755)

	// Test: script creates a file, then verify it ends up in the right place
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("echo 'test-data' > output.txt\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/var/run/act/workflow/0.sh"}, nil, dir, nil, "/app/src", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}

	// Verify the file was created in the workdir
	data, err := os.ReadFile(dir + "/app/src/output.txt")
	if err != nil {
		t.Fatalf("file not found in workdir: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "test-data" {
		t.Fatalf("got %q, want %q", got, "test-data")
	}
}

func TestCatWrittenFile(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Write a file to the container's filesystem
	os.WriteFile(dir+"/tmp/hello.txt", []byte("file content\n"), 0644)

	var stdout bytes.Buffer
	code, err := rt.RunCommand(ctx, []string{"cat", "/tmp/hello.txt"}, nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "file content" {
		t.Fatalf("got %q, want %q", got, "file content")
	}
}

func TestProcess(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	proc, err := NewProcess(rt, []string{"echo", "from process"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	code := proc.Wait()
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}

	logs := string(proc.LogBytes())
	if !strings.Contains(logs, "from process") {
		t.Fatalf("logs missing output: %q", logs)
	}
}

func TestProcessExec(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	// Start a long-running process
	proc, err := NewProcess(rt, []string{"sleep", "10"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	// Exec a command in the same container
	var stdout bytes.Buffer
	code := proc.RunExec(ctx, []string{"echo", "from exec"}, nil, "", nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "from exec" {
		t.Fatalf("got %q, want %q", got, "from exec")
	}

	// Signal to stop
	proc.Signal()
}

func TestBashOPipefail(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("echo 'pipefail works'\n"), 0755)

	// bash --noprofile --norc -e -o pipefail /path/to/script
	// This is the format act uses for bash shell steps.
	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx,
		[]string{"bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "/var/run/act/workflow/0.sh"},
		nil, dir, nil, "", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "pipefail works" {
		t.Fatalf("got %q, want %q", got, "pipefail works")
	}
}

func TestCatInWorkDir(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.MkdirAll(dir+"/tmp/workdir", 0755)
	os.WriteFile(dir+"/tmp/workdir/data.txt", []byte("hello-from-workdir\n"), 0644)

	// Run a script that reads data.txt via cat using a relative path,
	// verifying WASM CWD-aware path resolution.
	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("cat data.txt\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/var/run/act/workflow/0.sh"}, nil, dir, nil, "/tmp/workdir", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello-from-workdir" {
		t.Fatalf("got %q, want %q", got, "hello-from-workdir")
	}
}

// TestWorkDirRoundTrip tests the full working-directory scenario from E2E:
// write a file via redirect, then verify it with test -f and cat.
func TestWorkDirRoundTrip(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)
	os.MkdirAll(dir+"/tmp/workdir", 0755)
	os.MkdirAll(dir+"/var/run/act/workflow", 0755)

	// Step 1: write a file via redirect in workdir
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("echo 'test-content' > output.txt\n"), 0755)
	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/var/run/act/workflow/0.sh"}, nil, dir, nil, "/tmp/workdir", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("step 1 exit code %d, stderr: %s", code, stderr.String())
	}

	// Step 2: verify with test -f and cat (same as working-dir E2E workflow)
	os.WriteFile(dir+"/var/run/act/workflow/1.sh",
		[]byte("test -f output.txt\ntest \"$(cat output.txt)\" = \"test-content\"\necho 'Working directory test passed'\n"), 0755)
	stdout.Reset()
	stderr.Reset()
	code, err = rt.RunSimpleCommand(ctx, []string{"sh", "-e", "/var/run/act/workflow/1.sh"}, nil, dir, nil, "/tmp/workdir", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("step 2 exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "Working directory test passed" {
		t.Fatalf("got %q, want %q", got, "Working directory test passed")
	}
}

func TestDateBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, err := rt.RunShell(ctx, "date", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		t.Fatal("date produced no output")
	}
	// Should contain UTC and a year
	if !strings.Contains(out, "UTC") {
		t.Fatalf("date output missing UTC: %q", out)
	}
}

func TestUnameBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "uname", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "Linux" {
		t.Fatalf("got %q, want %q", got, "Linux")
	}

	stdout.Reset()
	code, _ = rt.RunShell(ctx, "uname -a", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "Linux sandbox") {
		t.Fatalf("uname -a output unexpected: %q", out)
	}
}

func TestMktempBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// mktemp (file)
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "mktemp", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	path := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(path, "/tmp/tmp.") {
		t.Fatalf("unexpected path: %q", path)
	}
	// Verify the file exists on the host
	if _, err := os.Stat(dir + path); err != nil {
		t.Fatalf("temp file not created: %v", err)
	}
}

func TestMktempDirBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// mktemp -d (directory)
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "mktemp -d", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	path := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(path, "/tmp/tmp.") {
		t.Fatalf("unexpected path: %q", path)
	}
	info, err := os.Stat(dir + path)
	if err != nil {
		t.Fatalf("temp dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("mktemp -d did not create a directory")
	}
}

func TestIdBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Default (root)
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "id", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "uid=0(root) gid=0(root)" {
		t.Fatalf("got %q", got)
	}

	// With custom UID/GID
	stdout.Reset()
	env := []string{"SOCKERLESS_UID=100", "SOCKERLESS_GID=101"}
	code, _ = rt.RunShell(ctx, "id -u", env, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "100" {
		t.Fatalf("got %q, want 100", got)
	}

	stdout.Reset()
	code, _ = rt.RunShell(ctx, "id -g", env, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "101" {
		t.Fatalf("got %q, want 101", got)
	}
}

func TestUnameFlags(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	tests := []struct {
		cmd  string
		env  []string
		want string
	}{
		{"uname -s", nil, "Linux"},
		{"uname -n", nil, "sandbox"},
		{"uname -n", []string{"HOSTNAME=myhost"}, "myhost"},
		{"uname -r", nil, "5.15.0"},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		code, _ := rt.RunShell(ctx, tt.cmd, tt.env, dir, nil, nil, &stdout, &bytes.Buffer{})
		if code != 0 {
			t.Fatalf("%s: exit code %d", tt.cmd, code)
		}
		if got := strings.TrimSpace(stdout.String()); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestHostnameBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Default
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "hostname", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "localhost" {
		t.Fatalf("got %q, want localhost", got)
	}

	// With HOSTNAME env
	stdout.Reset()
	code, _ = rt.RunShell(ctx, "hostname", []string{"HOSTNAME=myhost"}, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "myhost" {
		t.Fatalf("got %q, want myhost", got)
	}
}

func TestMvCrossMount(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Create a volume mount in a separate temp dir to simulate cross-device
	volDir := t.TempDir()
	os.WriteFile(volDir+"/data.txt", []byte("cross-mount\n"), 0644)

	mounts := []DirMount{{HostPath: volDir, ContainerPath: "/vol"}}

	// Move file from volume to container filesystem (cross-device)
	var stdout, stderr bytes.Buffer
	code, err := rt.RunShell(ctx, "mv /vol/data.txt /tmp/moved.txt && cat /tmp/moved.txt",
		nil, dir, mounts, nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "cross-mount" {
		t.Fatalf("got %q, want %q", got, "cross-mount")
	}
}

func TestBashEnvVars(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// When invoked as "bash -c", $BASH and $SHELL should be set
	var stdout bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx, []string{"bash", "-c", "echo BASH=$BASH SHELL=$SHELL"}, nil, dir, nil, "", nil, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.Contains(out, "BASH=/bin/bash") {
		t.Fatalf("missing BASH var: %q", out)
	}
	if !strings.Contains(out, "SHELL=/bin/bash") {
		t.Fatalf("missing SHELL var: %q", out)
	}

	// When invoked as "sh -c", $BASH should NOT be set
	stdout.Reset()
	code, _ = rt.RunSimpleCommand(ctx, []string{"sh", "-c", "echo BASH=${BASH:-unset}"}, nil, dir, nil, "", nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out = strings.TrimSpace(stdout.String())
	if !strings.Contains(out, "BASH=unset") {
		t.Fatalf("BASH should not be set for sh: %q", out)
	}
}

func TestProcessStdinPipe(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	// Start a shell that reads from stdin
	proc, err := NewProcess(rt, []string{"sh"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	// Write a script to stdin, then close to signal EOF
	sw := proc.StdinWriter()
	_, err = sw.Write([]byte("echo hello-from-stdin\n"))
	if err != nil {
		t.Fatal(err)
	}
	sw.Close()

	code := proc.Wait()
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}

	logs := string(proc.LogBytes())
	if !strings.Contains(logs, "hello-from-stdin") {
		t.Fatalf("logs missing stdin output: %q", logs)
	}
}

func TestProcessStdinEOF(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	// Start a shell that reads from stdin
	proc, err := NewProcess(rt, []string{"sh"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	// Write partial input and close — shell should exit on EOF
	sw := proc.StdinWriter()
	_, _ = sw.Write([]byte("echo partial\n"))
	sw.Close()

	code := proc.Wait()
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}

	logs := string(proc.LogBytes())
	if !strings.Contains(logs, "partial") {
		t.Fatalf("logs missing output: %q", logs)
	}
}

func TestProcessStdinWithExec(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	// Start a shell reading from stdin (long-running via sleep after stdin commands)
	proc, err := NewProcess(rt, []string{"sh", "-c", "sleep 5"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	// Run an exec concurrently — should work even though main process has stdin attached
	var stdout bytes.Buffer
	code := proc.RunExec(ctx, []string{"echo", "from-exec"}, nil, "", nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exec exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "from-exec" {
		t.Fatalf("got %q, want %q", got, "from-exec")
	}

	proc.Signal()
}

func TestPATHScriptOverride(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Create a custom "ls" script in /usr/local/bin that outputs a specific string
	if err := os.WriteFile(dir+"/usr/local/bin/ls", []byte("echo custom-ls-output\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Run "ls" with PATH that includes /usr/local/bin first
	// The custom script should run instead of the busybox ls applet
	var stdout bytes.Buffer
	env := []string{"PATH=/usr/local/bin:/bin"}
	code, err := rt.RunShell(ctx, "ls", env, dir, nil, nil, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "custom-ls-output" {
		t.Fatalf("expected custom script output, got %q", got)
	}
}

// TestDefaultsRunPwdCheck replicates the upstream act "defaults-run" test:
// bash -e -o pipefail script.sh with workdir=/tmp, script does [ $(pwd) = /tmp ] || exit 2
func TestDefaultsRunPwdCheck(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Write the script
	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("echo $SHELL | grep bash || exit 1\n[ $(pwd) = /tmp ] || exit 2\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx,
		[]string{"bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "/var/run/act/workflow/0.sh"},
		[]string{"SHELL=/bin/bash"}, dir, nil, "/tmp", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stdout: %q", stdout.String())
	t.Logf("stderr: %q", stderr.String())
	if code != 0 {
		t.Fatalf("exit code %d (want 0), stderr: %s", code, stderr.String())
	}
}

// TestWorkdirPwdBashTest replicates the upstream act "workdir" test:
// bash -e script that does [[ "$(pwd)" == "/tmp" ]] with workdir=/tmp
func TestWorkdirPwdBashTest(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Write the script content: [[ "$(pwd)" == "/tmp" ]]
	os.MkdirAll(dir+"/var/run/act/workflow", 0755)
	os.WriteFile(dir+"/var/run/act/workflow/0.sh",
		[]byte("[[ \"$(pwd)\" == \"/tmp\" ]]\n"), 0755)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx,
		[]string{"bash", "-e", "/var/run/act/workflow/0.sh"},
		nil, dir, nil, "/tmp", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stdout: %q", stdout.String())
	t.Logf("stderr: %q", stderr.String())
	if code != 0 {
		t.Fatalf("exit code %d (want 0), stderr: %s", code, stderr.String())
	}
}

// TestPwdInCommandSubstitution tests that $(pwd) returns correct container-relative path
func TestPwdInCommandSubstitution(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx,
		[]string{"sh", "-c", "echo $(pwd)"},
		nil, dir, nil, "/tmp", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != "/tmp" {
		t.Fatalf("$(pwd) returned %q, want %q", got, "/tmp")
	}
}

// TestPwdStandaloneWithWorkDir tests standalone pwd with workDir
func TestPwdStandaloneWithWorkDir(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, err := rt.RunSimpleCommand(ctx,
		[]string{"sh", "-c", "pwd"},
		nil, dir, nil, "/tmp", nil, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	t.Logf("standalone pwd returned: %q", got)
	if got != "/tmp" {
		t.Fatalf("pwd returned %q, want %q", got, "/tmp")
	}
}

func TestTouchBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout, stderr bytes.Buffer
	code, _ := rt.RunShell(ctx, "touch /tmp/newfile.txt && test -f /tmp/newfile.txt && echo ok",
		nil, dir, nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
}

func TestBase64Builtin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Encode
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "echo -n hello | base64", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("encode exit code %d", code)
	}
	encoded := strings.TrimSpace(stdout.String())
	if encoded != "aGVsbG8=" {
		t.Fatalf("encode got %q, want %q", encoded, "aGVsbG8=")
	}

	// Decode
	stdout.Reset()
	code, _ = rt.RunShell(ctx, "echo aGVsbG8= | base64 -d", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("decode exit code %d", code)
	}
	if got := stdout.String(); got != "hello" {
		t.Fatalf("decode got %q, want %q", got, "hello")
	}
}

func TestBasenameBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	tests := []struct {
		cmd  string
		want string
	}{
		{"basename /usr/local/bin/file.txt", "file.txt"},
		{"basename /usr/local/bin/file.txt .txt", "file"},
		{"basename /", "/"},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		code, _ := rt.RunShell(ctx, tt.cmd, nil, dir, nil, nil, &stdout, &bytes.Buffer{})
		if code != 0 {
			t.Fatalf("%s: exit code %d", tt.cmd, code)
		}
		if got := strings.TrimSpace(stdout.String()); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestDirnameBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	tests := []struct {
		cmd  string
		want string
	}{
		{"dirname /usr/local/bin/file.txt", "/usr/local/bin"},
		{"dirname /usr/local/bin/", "/usr/local"},
		{"dirname file.txt", "."},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		code, _ := rt.RunShell(ctx, tt.cmd, nil, dir, nil, nil, &stdout, &bytes.Buffer{})
		if code != 0 {
			t.Fatalf("%s: exit code %d", tt.cmd, code)
		}
		if got := strings.TrimSpace(stdout.String()); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestWhichBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Known applet
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "which cat", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "/bin/cat" {
		t.Fatalf("got %q, want /bin/cat", got)
	}

	// Custom PATH script
	stdout.Reset()
	os.WriteFile(dir+"/usr/local/bin/myscript", []byte("echo hi\n"), 0755)
	env := []string{"PATH=/usr/local/bin:/bin"}
	code, _ = rt.RunShell(ctx, "which myscript", env, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "/usr/local/bin/myscript" {
		t.Fatalf("got %q, want /usr/local/bin/myscript", got)
	}

	// Not found
	var stderr bytes.Buffer
	code, _ = rt.RunShell(ctx, "which nonexistent", nil, dir, nil, nil, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for nonexistent command")
	}
}

func TestSeqBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	tests := []struct {
		cmd  string
		want string
	}{
		{"seq 3", "1\n2\n3"},
		{"seq 2 5", "2\n3\n4\n5"},
		{"seq 1 2 7", "1\n3\n5\n7"},
	}
	for _, tt := range tests {
		var stdout bytes.Buffer
		code, _ := rt.RunShell(ctx, tt.cmd, nil, dir, nil, nil, &stdout, &bytes.Buffer{})
		if code != 0 {
			t.Fatalf("%s: exit code %d", tt.cmd, code)
		}
		if got := strings.TrimSpace(stdout.String()); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestReadlinkBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	// Create a symlink
	os.Symlink("/tmp/target", dir+"/tmp/mylink")
	os.WriteFile(dir+"/tmp/target", []byte("data"), 0644)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "readlink /tmp/mylink", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "/tmp/target" {
		t.Fatalf("got %q, want /tmp/target", got)
	}

	// readlink -f (canonical)
	stdout.Reset()
	code, _ = rt.RunShell(ctx, "readlink -f /tmp/mylink", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("readlink -f exit code %d", code)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "/tmp/target" {
		t.Fatalf("readlink -f got %q, want /tmp/target", got)
	}
}

func TestTeeBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "echo hello | tee /tmp/tee-out.txt", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	// Stdout should have the data
	if got := strings.TrimSpace(stdout.String()); got != "hello" {
		t.Fatalf("stdout got %q, want hello", got)
	}
	// File should also have the data
	data, err := os.ReadFile(dir + "/tmp/tee-out.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "hello" {
		t.Fatalf("file got %q, want hello", got)
	}
}

func TestLnBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.WriteFile(dir+"/tmp/original.txt", []byte("original"), 0644)

	// Symbolic link
	var stderr bytes.Buffer
	code, _ := rt.RunShell(ctx, "ln -s /tmp/original.txt /tmp/symlink.txt", nil, dir, nil, nil, &bytes.Buffer{}, &stderr)
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	target, err := os.Readlink(dir + "/tmp/symlink.txt")
	if err != nil {
		t.Fatal(err)
	}
	if target != "/tmp/original.txt" {
		t.Fatalf("symlink target %q, want /tmp/original.txt", target)
	}

	// Force overwrite
	code, _ = rt.RunShell(ctx, "ln -sf /tmp/original.txt /tmp/symlink.txt", nil, dir, nil, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("force ln exit code %d", code)
	}
}

func TestStatBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.WriteFile(dir+"/tmp/statfile.txt", []byte("12345"), 0644)

	// stat -c %s (size)
	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "stat -c %s /tmp/statfile.txt", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "5" {
		t.Fatalf("stat -c %%s got %q, want 5", got)
	}

	// stat -c %n (name)
	stdout.Reset()
	code, _ = rt.RunShell(ctx, "stat -c %n /tmp/statfile.txt", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "statfile.txt" {
		t.Fatalf("stat -c %%n got %q, want statfile.txt", got)
	}
}

func TestSha256sumBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.WriteFile(dir+"/tmp/hashfile.txt", []byte("hello"), 0644)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "sha256sum /tmp/hashfile.txt", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	// SHA256 of "hello" is 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	if !strings.HasPrefix(out, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824") {
		t.Fatalf("sha256sum got %q", out)
	}

	// From stdin
	stdout.Reset()
	code, _ = rt.RunShell(ctx, "echo -n test | sha256sum", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("stdin exit code %d", code)
	}
	out = strings.TrimSpace(stdout.String())
	// SHA256 of "test" is 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
	if !strings.HasPrefix(out, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08") {
		t.Fatalf("sha256sum stdin got %q", out)
	}
}

func TestMd5sumBuiltin(t *testing.T) {
	ctx := context.Background()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	dir := t.TempDir()
	PopulateRootfs(dir)

	os.WriteFile(dir+"/tmp/md5file.txt", []byte("hello"), 0644)

	var stdout bytes.Buffer
	code, _ := rt.RunShell(ctx, "md5sum /tmp/md5file.txt", nil, dir, nil, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	out := strings.TrimSpace(stdout.String())
	// MD5 of "hello" is 5d41402abc4b2a76b9719d911017c592
	if !strings.HasPrefix(out, "5d41402abc4b2a76b9719d911017c592") {
		t.Fatalf("md5sum got %q", out)
	}
}

func TestPopulateRootfs(t *testing.T) {
	dir := t.TempDir()
	if err := PopulateRootfs(dir); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"bin", "sbin", "usr/bin", "etc", "tmp", "var/log",
		"etc/passwd", "etc/group", "etc/hostname", "etc/hosts", "etc/resolv.conf",
	} {
		if _, err := os.Stat(dir + "/" + path); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}
