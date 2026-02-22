package sandbox

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"
)

// builtinTouch creates files or updates their timestamps.
func builtinTouch(args []string, hc interp.HandlerContext, rootDir string) error {
	for _, arg := range args[1:] {
		if arg == "" || arg[0] == '-' {
			continue
		}
		p := resolvePathForBuiltin(arg, hc, rootDir)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "touch: %v\n", err)
			return interp.ExitStatus(1)
		}
		_ = f.Close()
		now := time.Now()
		_ = os.Chtimes(p, now, now)
	}
	return nil
}

// builtinTee duplicates stdin to stdout and one or more files.
func builtinTee(args []string, hc interp.HandlerContext, rootDir string) error {
	appendMode := false
	var files []string
	for _, a := range args[1:] {
		if a == "-a" || a == "--append" {
			appendMode = true
		} else if a == "" || a[0] == '-' {
			continue
		} else {
			files = append(files, a)
		}
	}
	var writers []io.Writer
	writers = append(writers, hc.Stdout)
	var closers []io.Closer
	for _, f := range files {
		p := resolvePathForBuiltin(f, hc, rootDir)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		flag := os.O_CREATE | os.O_WRONLY
		if appendMode {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}
		fh, err := os.OpenFile(p, flag, 0644)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "tee: %v\n", err)
			return interp.ExitStatus(1)
		}
		writers = append(writers, fh)
		closers = append(closers, fh)
	}
	mw := io.MultiWriter(writers...)
	_, _ = io.Copy(mw, hc.Stdin)
	for _, c := range closers {
		_ = c.Close()
	}
	return nil
}

// builtinLn creates hard or symbolic links.
func builtinLn(args []string, hc interp.HandlerContext, rootDir string) error {
	symbolic := false
	force := false
	var operands []string
	for _, a := range args[1:] {
		switch a {
		case "-s", "--symbolic":
			symbolic = true
		case "-f", "--force":
			force = true
		case "-sf", "-fs":
			symbolic = true
			force = true
		default:
			if a != "" && a[0] != '-' {
				operands = append(operands, a)
			}
		}
	}
	if len(operands) < 2 {
		fmt.Fprintf(hc.Stderr, "ln: missing operand\n")
		return interp.ExitStatus(1)
	}
	target := operands[0]
	linkName := resolvePathForBuiltin(operands[1], hc, rootDir)
	if force {
		_ = os.Remove(linkName)
	}
	if symbolic {
		if err := os.Symlink(target, linkName); err != nil {
			fmt.Fprintf(hc.Stderr, "ln: %v\n", err)
			return interp.ExitStatus(1)
		}
	} else {
		targetPath := resolvePathForBuiltin(target, hc, rootDir)
		if err := os.Link(targetPath, linkName); err != nil {
			fmt.Fprintf(hc.Stderr, "ln: %v\n", err)
			return interp.ExitStatus(1)
		}
	}
	return nil
}

// builtinStat prints file status information.
func builtinStat(args []string, hc interp.HandlerContext, rootDir string) error {
	format := ""
	var targets []string
	for i := 1; i < len(args); i++ {
		if args[i] == "-c" && i+1 < len(args) {
			i++
			format = args[i]
		} else if args[i] != "" && args[i][0] != '-' {
			targets = append(targets, args[i])
		}
	}
	for _, target := range targets {
		p := resolvePathForBuiltin(target, hc, rootDir)
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "stat: cannot stat '%s': %v\n", target, err)
			return interp.ExitStatus(1)
		}
		if format != "" {
			out := format
			out = strings.ReplaceAll(out, "%s", strconv.FormatInt(info.Size(), 10))
			out = strings.ReplaceAll(out, "%n", info.Name())
			out = strings.ReplaceAll(out, "%F", fileTypeString(info))
			fmt.Fprintln(hc.Stdout, out)
		} else {
			fmt.Fprintf(hc.Stdout, "  File: %s\n  Size: %d\n", target, info.Size())
		}
	}
	return nil
}

// builtinSha256sum computes SHA-256 checksums of files or stdin.
func builtinSha256sum(args []string, hc interp.HandlerContext, rootDir string) error {
	var files []string
	for _, a := range args[1:] {
		if a != "" && a[0] != '-' {
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		// Read from stdin
		data, _ := io.ReadAll(hc.Stdin)
		h := sha256.Sum256(data)
		fmt.Fprintf(hc.Stdout, "%x  -\n", h)
	} else {
		for _, f := range files {
			p := resolvePathForBuiltin(f, hc, rootDir)
			data, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "sha256sum: %s: %v\n", f, err)
				return interp.ExitStatus(1)
			}
			h := sha256.Sum256(data)
			fmt.Fprintf(hc.Stdout, "%x  %s\n", h, f)
		}
	}
	return nil
}

// builtinMd5sum computes MD5 checksums of files or stdin.
func builtinMd5sum(args []string, hc interp.HandlerContext, rootDir string) error {
	var files []string
	for _, a := range args[1:] {
		if a != "" && a[0] != '-' {
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		data, _ := io.ReadAll(hc.Stdin)
		h := md5.Sum(data)
		fmt.Fprintf(hc.Stdout, "%x  -\n", h)
	} else {
		for _, f := range files {
			p := resolvePathForBuiltin(f, hc, rootDir)
			data, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "md5sum: %s: %v\n", f, err)
				return interp.ExitStatus(1)
			}
			h := md5.Sum(data)
			fmt.Fprintf(hc.Stdout, "%x  %s\n", h, f)
		}
	}
	return nil
}

// builtinReadlink reads the target of a symbolic link.
func builtinReadlink(args []string, hc interp.HandlerContext, rootDir string) error {
	canonicalize := false
	var targets []string
	for _, a := range args[1:] {
		if a == "-f" || a == "--canonicalize" {
			canonicalize = true
		} else if a == "" || a[0] == '-' {
			continue
		} else {
			targets = append(targets, a)
		}
	}
	for _, target := range targets {
		p := resolvePathForBuiltin(target, hc, rootDir)
		var result string
		if canonicalize {
			// Manually resolve symlinks step by step within rootDir.
			// filepath.EvalSymlinks would follow absolute symlink
			// targets on the HOST, but we need to resolve them
			// inside the container's rootDir.
			cur := p
			for i := 0; i < 40; i++ { // max symlink depth
				link, err := os.Readlink(cur)
				if err != nil {
					break // not a symlink
				}
				if filepath.IsAbs(link) {
					cur = filepath.Join(rootDir, link)
				} else {
					cur = filepath.Join(filepath.Dir(cur), link)
				}
			}
			result = cur
		} else {
			link, err := os.Readlink(p)
			if err != nil {
				fmt.Fprintf(hc.Stderr, "readlink: %s: %v\n", target, err)
				return interp.ExitStatus(1)
			}
			result = link
		}
		// Strip rootDir prefix for container-relative output
		if strings.HasPrefix(result, rootDir) {
			result = strings.TrimPrefix(result, rootDir)
			if result == "" {
				result = "/"
			}
		}
		fmt.Fprintln(hc.Stdout, result)
	}
	return nil
}

// builtinMktemp creates temporary files or directories.
func builtinMktemp(args []string, hc interp.HandlerContext, rootDir string) error {
	dirMode := false
	parentDir := filepath.Join(rootDir, "tmp")
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-d":
			dirMode = true
		case "-p":
			if i+1 < len(args) {
				i++
				p := args[i]
				if filepath.IsAbs(p) {
					p = filepath.Join(rootDir, p)
				} else {
					p = filepath.Join(hc.Dir, p)
				}
				parentDir = p
			}
		}
	}
	_ = os.MkdirAll(parentDir, 0755)
	if dirMode {
		d, err := os.MkdirTemp(parentDir, "tmp.*")
		if err != nil {
			fmt.Fprintf(hc.Stderr, "mktemp: %v\n", err)
			return interp.ExitStatus(1)
		}
		rel := strings.TrimPrefix(d, rootDir)
		fmt.Fprintln(hc.Stdout, rel)
	} else {
		f, err := os.CreateTemp(parentDir, "tmp.*")
		if err != nil {
			fmt.Fprintf(hc.Stderr, "mktemp: %v\n", err)
			return interp.ExitStatus(1)
		}
		_ = f.Close()
		rel := strings.TrimPrefix(f.Name(), rootDir)
		fmt.Fprintln(hc.Stdout, rel)
	}
	return nil
}
