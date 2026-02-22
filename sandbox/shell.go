package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// RunShell parses a shell command string and executes it, dispatching
// individual commands to WASM busybox applets. Supports pipes, &&, ||,
// variable expansion, redirects, and command substitution.
func (r *Runtime) RunShell(ctx context.Context, command string, env []string,
	rootDir string, mounts []DirMount, stdin io.Reader, stdout, stderr io.Writer) (exitCode int, err error) {
	return r.runShellInDir(ctx, command, env, rootDir, rootDir, mounts, stdin, stdout, stderr)
}

// runShellInDir is like RunShell but allows specifying the initial working
// directory (as a host path). The shellDir must be under rootDir.
func (r *Runtime) runShellInDir(ctx context.Context, command string, env []string,
	rootDir, shellDir string, mounts []DirMount, stdin io.Reader, stdout, stderr io.Writer) (exitCode int, err error) {

	// Prepend PWD override so that the shell builtin pwd returns the
	// container-relative path instead of the host path. mvdan.cc/sh's
	// pwd builtin reads the PWD variable, which New() initialises to
	// r.Dir (the host path). Overriding it with a shell assignment is
	// the only reliable way to fix this, since pwd is a shell builtin
	// and cannot be intercepted via ExecHandlers.
	containerDir := strings.TrimPrefix(shellDir, rootDir)
	if containerDir == "" {
		containerDir = "/"
	}
	command = "PWD=" + shellQuote(containerDir) + "\n" + command

	prog, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		fmt.Fprintf(stderr, "sh: syntax error: %v\n", err)
		return 2, nil
	}

	runner, err := r.newRunner(ctx, env, rootDir, shellDir, mounts, stdin, stdout, stderr)
	if err != nil {
		return 1, err
	}

	err = runner.Run(ctx, prog)
	return getExitCode(err), nil
}

// shellQuote wraps a string in single quotes for safe shell embedding.
func shellQuote(s string) string {
	// If it contains no special characters, return as-is
	if !strings.ContainsAny(s, " \t\n'\"\\$`!&|;(){}[]<>*?~#") {
		return s
	}
	// Single-quote and escape any embedded single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// RunInteractiveShell starts a REPL that reads lines from stdin and
// executes them. Each line is parsed and dispatched through the shell
// interpreter.
func (r *Runtime) RunInteractiveShell(ctx context.Context, env []string,
	rootDir string, mounts []DirMount, stdin io.Reader, stdout, stderr io.Writer) error {

	runner, err := r.newRunner(ctx, env, rootDir, rootDir, mounts, stdin, stdout, stderr)
	if err != nil {
		return err
	}

	parser := syntax.NewParser()
	fmt.Fprint(stdout, "/ # ")

	return parser.Interactive(stdin, func(stmts []*syntax.Stmt) bool {
		for _, stmt := range stmts {
			if err := runner.Run(ctx, stmt); err != nil {
				var exitErr interp.ExitStatus
				if !errors.As(err, &exitErr) {
					fmt.Fprintf(stderr, "sh: %v\n", err)
				}
			}
		}
		fmt.Fprint(stdout, "/ # ")
		return true
	})
}

func (r *Runtime) newRunner(ctx context.Context, env []string,
	rootDir, shellDir string, mounts []DirMount, stdin io.Reader, stdout, stderr io.Writer) (*interp.Runner, error) {

	// Rewrite container-absolute paths to be under rootDir so that shell
	// redirects (e.g. > /cache/file.txt) operate inside the container's
	// virtual filesystem rather than the host root. Paths that are already
	// under rootDir (resolved by the shell interpreter against its Dir)
	// are left unchanged.
	rewritePath := func(path string) string {
		if filepath.IsAbs(path) {
			if strings.HasPrefix(path, rootDir+"/") || path == rootDir {
				return path
			}
			return filepath.Join(rootDir, path)
		}
		return path
	}

	defaultOpen := interp.DefaultOpenHandler()
	openHandler := func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		return defaultOpen(ctx, rewritePath(path), flag, perm)
	}

	defaultStat := interp.DefaultStatHandler()
	statHandler := func(ctx context.Context, name string, followSymlinks bool) (os.FileInfo, error) {
		return defaultStat(ctx, rewritePath(name), followSymlinks)
	}

	return interp.New(
		interp.StdIO(stdin, stdout, stderr),
		interp.ExecHandlers(r.wasmExecHandler(rootDir, mounts)),
		interp.Env(expand.ListEnviron(env...)),
		interp.Dir(shellDir),
		interp.OpenHandler(openHandler),
		interp.StatHandler(statHandler),
	)
}

func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr interp.ExitStatus
	if errors.As(err, &exitErr) {
		return int(exitErr)
	}
	return 1
}

// CaptureOutput runs a command and returns its stdout as bytes.
func (r *Runtime) CaptureOutput(ctx context.Context, args []string, env []string,
	rootDir string, mounts []DirMount) ([]byte, int, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := r.RunSimpleCommand(ctx, args, env, rootDir, mounts, "", nil, &stdout, &stderr)
	return stdout.Bytes(), exitCode, err
}
