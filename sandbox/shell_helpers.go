package sandbox

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

// knownApplets is the set of busybox applets we dispatch to WASM.
// Only include applets that are actually compiled into busybox.wasm.
var knownApplets = map[string]bool{
	"ash": true, "awk": true, "cat": true, "cp": true, "cut": true,
	"diff": true, "dig": true, "echo": true,
	"false": true, "find": true, "free": true, "grep": true, "gunzip": true,
	"gzip": true, "head": true, "ionice": true, "kill": true,
	"killall": true, "logname": true, "ls": true, "mkdir": true, "mv": true,
	"nc": true, "nice": true, "nohup": true, "nproc": true, "pgrep": true,
	"pidof": true, "pkill": true, "printf": true, "ps": true, "pwd": true,
	"renice": true, "rm": true, "rmdir": true, "sed": true, "setsid": true,
	"sh": true, "sleep": true, "sort": true, "ss": true,
	"start-stop-daemon": true, "tail": true, "tar": true, "taskset": true,
	"test": true, "time": true, "timeout": true, "top": true, "tr": true,
	"true": true, "uniq": true, "uptime": true, "users": true, "w": true,
	"watch": true, "wc": true, "wget": true, "who": true, "whoami": true,
	"xargs": true,
}

// fileArgCommands are commands whose non-flag arguments are primarily
// file paths. For these commands, relative arguments are always resolved
// to the container CWD (even for files that don't exist yet, like cp
// destinations). Other commands use an os.Stat check to avoid mangling
// non-path arguments (e.g. "echo hello").
var fileArgCommands = map[string]bool{
	"cat": true, "cp": true, "mv": true, "rm": true, "rmdir": true,
	"mkdir": true, "ls": true, "head": true, "tail": true, "diff": true,
	"gzip": true, "gunzip": true, "tar": true, "wc": true, "sort": true,
	"uniq": true, "find": true, "touch": true,
	"readlink": true, "tee": true, "ln": true, "stat": true,
	"sha256sum": true, "md5sum": true,
}

// builtinCommands are commands implemented directly in the shell handler.
var builtinCommands = map[string]bool{
	"chmod": true, "chown": true, "mktemp": true,
	"hostname": true, "id": true,
	"touch": true, "base64": true, "basename": true, "dirname": true,
	"which": true, "seq": true, "readlink": true, "tee": true,
	"ln": true, "stat": true, "sha256sum": true, "md5sum": true,
}

// mvFallback handles cross-device mv by copying then removing the source.
// Container paths are resolved through rootDir and volume mounts.
// Returns 0 on success, 1 on failure.
func mvFallback(args []string, rootDir string, mounts []DirMount) int {
	// Extract non-flag args
	var paths []string
	for _, a := range args[1:] {
		if a != "" && a[0] != '-' {
			paths = append(paths, a)
		}
	}
	if len(paths) != 2 {
		return 1
	}
	src := resolveContainerPath(paths[0], rootDir, mounts)
	dst := resolveContainerPath(paths[1], rootDir, mounts)

	info, err := os.Stat(src)
	if err != nil {
		return 1
	}

	// Try os.Rename first (works if same device)
	if err := os.Rename(src, dst); err == nil {
		return 0
	}

	if info.IsDir() {
		return 1
	}

	// Copy file then remove source
	in, err := os.Open(src)
	if err != nil {
		return 1
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return 1
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return 1
	}
	_ = out.Close()
	_ = in.Close()
	if err := os.Remove(src); err != nil {
		return 1
	}
	return 0
}

// resolveContainerPath converts a container-absolute path to its host
// equivalent, checking volume mounts first then falling back to rootDir.
func resolveContainerPath(containerPath string, rootDir string, mounts []DirMount) string {
	containerPath = filepath.Clean(containerPath)
	for _, m := range mounts {
		cp := filepath.Clean(m.ContainerPath)
		if containerPath == cp || strings.HasPrefix(containerPath, cp+"/") {
			rel := strings.TrimPrefix(containerPath, cp)
			return filepath.Join(m.HostPath, rel)
		}
	}
	return filepath.Join(rootDir, containerPath)
}

// findInPATH looks for a script file named cmdName in the PATH directories,
// searching within the container's rootDir. Returns the host path to the
// script file if found, or empty string if not found. This allows user scripts
// to override busybox builtins when prepended to PATH.
func findInPATH(cmdName string, hc interp.HandlerContext, rootDir string) string {
	// Don't override for absolute paths
	if strings.Contains(cmdName, "/") {
		return ""
	}
	pathVar := ""
	hc.Env.Each(func(name string, vr expand.Variable) bool {
		if name == "PATH" && vr.Kind == expand.String {
			pathVar = vr.Str
			return false
		}
		return true
	})
	if pathVar == "" {
		return ""
	}
	for _, dir := range strings.Split(pathVar, ":") {
		if dir == "" {
			continue
		}
		// Resolve container path to host path
		hostPath := filepath.Join(rootDir, filepath.Clean(dir), cmdName)
		info, err := os.Stat(hostPath)
		if err == nil && !info.IsDir() {
			return hostPath
		}
	}
	return ""
}

func isBashBinary(name string) bool {
	return name == "bash" || name == "/bin/bash"
}

// injectBashEnv adds BASH and SHELL env vars when running under bash,
// unless they are already set.
func injectBashEnv(env []string) []string {
	hasBASH, hasSHELL := false, false
	for _, e := range env {
		if strings.HasPrefix(e, "BASH=") {
			hasBASH = true
		} else if strings.HasPrefix(e, "SHELL=") {
			hasSHELL = true
		}
	}
	if !hasBASH {
		env = append(env, "BASH=/bin/bash")
	}
	if !hasSHELL {
		env = append(env, "SHELL=/bin/bash")
	}
	return env
}
