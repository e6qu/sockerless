package sandbox

import (
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/interp"
)

// resolvePathForBuiltin resolves a file path argument for a Go-implemented
// builtin. Absolute paths are resolved under rootDir; relative paths are
// resolved under the shell's current directory (hc.Dir).
func resolvePathForBuiltin(arg string, hc interp.HandlerContext, rootDir string) string {
	if filepath.IsAbs(arg) {
		if strings.HasPrefix(arg, rootDir+"/") || arg == rootDir {
			return arg
		}
		return filepath.Join(rootDir, arg)
	}
	return filepath.Join(hc.Dir, arg)
}

// fileTypeString returns a human-readable file type string for stat output.
func fileTypeString(info os.FileInfo) string {
	if info.IsDir() {
		return "directory"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symbolic link"
	}
	return "regular file"
}
