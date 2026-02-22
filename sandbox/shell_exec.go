package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

func (r *Runtime) wasmExecHandler(rootDir string, mounts []DirMount) func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return nil
			}

			name := args[0]

			// Handle builtins that the shell interpreter manages
			switch name {
			case "exit":
				code := 0
				if len(args) > 1 {
					fmt.Sscanf(args[1], "%d", &code)
				}
				return interp.ExitStatus(code)
			case "cd", "export", "unset", "set", "shift", "read", "eval", "source", ".":
				return next(ctx, args)
			case "pwd":
				return builtinPwd(args, interp.HandlerCtx(ctx), rootDir)
			case "date":
				return builtinDate(args, interp.HandlerCtx(ctx))
			case "uname":
				return builtinUname(args, interp.HandlerCtx(ctx))
			case "env":
				return builtinEnv(args, interp.HandlerCtx(ctx))
			case "chmod", "chown":
				return nil // No-op: WASI has no file permission model.
			case "mktemp":
				return builtinMktemp(args, interp.HandlerCtx(ctx), rootDir)
			case "hostname":
				return builtinHostname(args, interp.HandlerCtx(ctx))
			case "id":
				return builtinId(args, interp.HandlerCtx(ctx))
			case "touch":
				return builtinTouch(args, interp.HandlerCtx(ctx), rootDir)
			case "base64":
				return builtinBase64(args, interp.HandlerCtx(ctx))
			case "basename":
				return builtinBasename(args, interp.HandlerCtx(ctx))
			case "dirname":
				return builtinDirname(args, interp.HandlerCtx(ctx))
			case "which":
				return builtinWhich(args, interp.HandlerCtx(ctx), rootDir)
			case "seq":
				return builtinSeq(args, interp.HandlerCtx(ctx))
			case "readlink":
				return builtinReadlink(args, interp.HandlerCtx(ctx), rootDir)
			case "tee":
				return builtinTee(args, interp.HandlerCtx(ctx), rootDir)
			case "ln":
				return builtinLn(args, interp.HandlerCtx(ctx), rootDir)
			case "stat":
				return builtinStat(args, interp.HandlerCtx(ctx), rootDir)
			case "sha256sum":
				return builtinSha256sum(args, interp.HandlerCtx(ctx), rootDir)
			case "md5sum":
				return builtinMd5sum(args, interp.HandlerCtx(ctx), rootDir)
			}

			// Check PATH for a script file before falling back to busybox
			// applets. This allows user-defined scripts to override builtins
			// (e.g. a custom "ls" script prepended to PATH).
			hc := interp.HandlerCtx(ctx)
			if scriptPath := findInPATH(name, hc, rootDir); scriptPath != "" {
				data, err := os.ReadFile(scriptPath)
				if err == nil {
					exitCode, err := r.runShellInDir(ctx, string(data), nil, rootDir, hc.Dir, mounts, hc.Stdin, hc.Stdout, hc.Stderr)
					if err != nil {
						fmt.Fprintf(hc.Stderr, "sh: %s: %v\n", name, err)
						return interp.ExitStatus(1)
					}
					if exitCode != 0 {
						return interp.ExitStatus(exitCode)
					}
					return nil
				}
			}

			// Check if this is a known busybox applet
			if !knownApplets[name] {
				fmt.Fprintf(hc.Stderr, "sh: %s: not found\n", name)
				return interp.ExitStatus(127)
			}

			// Build environment from the shell's current state
			var envList []string
			hc.Env.Each(func(name string, vr expand.Variable) bool {
				if vr.Exported && vr.Kind == expand.String {
					envList = append(envList, name+"="+vr.Str)
				}
				return true
			})

			// Resolve relative path arguments when CWD differs from rootDir.
			// WASM has no CWD support; relative paths resolve to the rootDir.
			// Convert relative non-flag arguments to absolute container paths
			// so the WASM process operates in the correct directory.
			wasmArgs := args
			containerCWD := strings.TrimPrefix(hc.Dir, rootDir)
			if containerCWD == "" {
				containerCWD = "/"
			}
			if containerCWD != "/" {
				wasmArgs = make([]string, len(args))
				wasmArgs[0] = args[0]
				for i := 1; i < len(args); i++ {
					a := args[i]
					if a == "" || a[0] == '-' || a[0] == '/' {
						wasmArgs[i] = a
						continue
					}
					if fileArgCommands[name] {
						// For file-operating commands, always resolve
						// relative paths. This handles both reads and
						// writes (e.g. cp destination) correctly.
						wasmArgs[i] = filepath.Join(containerCWD, a)
					} else {
						// For other commands, only resolve if the file
						// actually exists (avoids mangling non-path
						// arguments like echo text).
						hostPath := filepath.Join(hc.Dir, a)
						if _, err := os.Stat(hostPath); err == nil {
							wasmArgs[i] = filepath.Join(containerCWD, a)
						} else {
							wasmArgs[i] = a
						}
					}
				}
			}

			exitCode, err := r.RunCommand(ctx, wasmArgs, envList, rootDir, mounts, hc.Stdin, hc.Stdout, hc.Stderr)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "sh: %s: %v\n", name, err)
				return interp.ExitStatus(1)
			}
			// mv cross-device fallback: WASM rename fails across mount boundaries,
			// fall back to Go-native copy+remove.
			if exitCode != 0 && name == "mv" {
				if code := mvFallback(wasmArgs, rootDir, mounts); code == 0 {
					return nil
				}
			}
			if exitCode != 0 {
				return interp.ExitStatus(exitCode)
			}
			return nil
		}
	}
}

// RunSimpleCommand runs a single command (no shell parsing). If the
// command is "sh" or "bash" with "-c", it delegates to RunShell.
// workDir is an optional container-relative working directory (e.g. "/app").
// It must be empty or an absolute path within the container.
func (r *Runtime) RunSimpleCommand(ctx context.Context, args []string, env []string,
	rootDir string, mounts []DirMount, workDir string,
	stdin io.Reader, stdout, stderr io.Writer) (int, error) {

	if len(args) == 0 {
		return 0, nil
	}

	// Resolve the effective initial directory for the shell.
	// We set Dir to rootDir/workDir (the host path) so that the shell
	// starts in the correct directory without needing a "cd" command
	// (which would fail because mvdan.cc/sh's access() check bypasses
	// our stat handler and checks the host filesystem directly).
	shellDir := rootDir
	if workDir != "" {
		shellDir = filepath.Join(rootDir, filepath.Clean(workDir))
	}

	name := args[0]
	if isShellBinary(name) {
		// Inject bash-specific env vars when invoked as bash
		if isBashBinary(name) {
			env = injectBashEnv(env)
		}

		// Parse shell flags and find the command/script
		flags := ""
		cmdIdx := -1
		scriptIdx := -1
		for i := 1; i < len(args); i++ {
			if args[i] == "-c" {
				cmdIdx = i
				break
			} else if args[i] == "-o" || args[i] == "+o" {
				// -o/-+o takes a following argument (e.g. -o pipefail)
				i++ // skip the option value
			} else if strings.HasPrefix(args[i], "-") || strings.HasPrefix(args[i], "+") {
				flags += strings.TrimLeft(args[i], "-+")
			} else {
				scriptIdx = i
				break
			}
		}

		prefix := ""
		if strings.Contains(flags, "e") {
			prefix = "set -e\n"
		}

		// "sh [-flags] -c 'command string'" → parse through shell
		if cmdIdx >= 0 && cmdIdx+1 < len(args) {
			cmd := strings.Join(args[cmdIdx+1:], " ")
			return r.runShellInDir(ctx, prefix+cmd, env, rootDir, shellDir, mounts, stdin, stdout, stderr)
		}

		// "sh [-flags] /path/to/script.sh" → read script file and run through shell
		if scriptIdx >= 0 {
			scriptPath := filepath.Join(rootDir, filepath.Clean(args[scriptIdx]))
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				fmt.Fprintf(stderr, "sh: can't open '%s': %v\n", args[scriptIdx], err)
				return 127, nil
			}
			return r.runShellInDir(ctx, prefix+string(data), env, rootDir, shellDir, mounts, stdin, stdout, stderr)
		}

		// "sh" with no args — read from stdin and execute
		if stdin != nil {
			script, err := io.ReadAll(stdin)
			if err != nil {
				fmt.Fprintf(stderr, "sh: read stdin: %v\n", err)
				return 1, nil
			}
			if len(script) > 0 {
				return r.runShellInDir(ctx, prefix+string(script), env, rootDir, shellDir, mounts, nil, stdout, stderr)
			}
		}
		return 0, nil
	}

	// For non-shell commands with workDir, wrap in a shell
	if workDir != "" {
		cmdStr := strings.Join(args, " ")
		return r.runShellInDir(ctx, cmdStr, env, rootDir, shellDir, mounts, stdin, stdout, stderr)
	}

	// Single command — check if it contains shell metacharacters
	cmdStr := strings.Join(args, " ")
	if containsShellMeta(cmdStr) {
		return r.RunShell(ctx, cmdStr, env, rootDir, mounts, stdin, stdout, stderr)
	}

	// Direct WASM execution — verify the command is a known applet
	if !knownApplets[args[0]] {
		fmt.Fprintf(stderr, "sh: %s: not found\n", args[0])
		return 127, nil
	}
	return r.RunCommand(ctx, args, env, rootDir, mounts, stdin, stdout, stderr)
}

func isShellBinary(name string) bool {
	switch name {
	case "sh", "/bin/sh", "bash", "/bin/bash", "ash", "/bin/ash":
		return true
	}
	return false
}

func containsShellMeta(s string) bool {
	return strings.ContainsAny(s, "|&;><$`\"'\\(){}*?[]!")
}

// IsShellCommand returns true if the command is a shell invocation.
func IsShellCommand(cmd []string) bool {
	if len(cmd) == 0 {
		return false
	}
	return isShellBinary(cmd[0])
}

// IsKnownCommand returns true if the command can be executed in the
// WASM sandbox (known applet, shell binary, or builtin).
func IsKnownCommand(name string) bool {
	return knownApplets[name] || isShellBinary(name) || builtinCommands[name]
}
