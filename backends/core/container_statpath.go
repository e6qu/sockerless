package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// RunContainerStatPathViaAgent implements `docker container stat <path>`
// by executing `stat` inside the container via the reverse-agent.
// The output format string is chosen so we can parse name, size, mode
// (octal), mtime epoch, and symlink target unambiguously.
func RunContainerStatPathViaAgent(reg *ReverseAgentRegistry, containerID, path string) (*api.ContainerPathStat, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	// Format: %n\t%s\t%f\t%Y\t%N — name, size, raw hex mode, mtime
	// seconds since epoch, name-with-symlink-target.
	argv := []string{"stat", "-c", `%n` + "\t" + `%s` + "\t" + `%f` + "\t" + `%Y` + "\t" + `%N`, path}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, "stat-"+containerID, argv, nil, "")
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		return nil, fmt.Errorf("stat failed (exit %d): %s", exit, msg)
	}
	return ParseStatOutput(strings.TrimSpace(string(stdout)), path)
}

// ParseStatOutput parses GNU-stat `%n\t%s\t%f\t%Y\t%N` into an
// api.ContainerPathStat.
func ParseStatOutput(raw, requestedPath string) (*api.ContainerPathStat, error) {
	fields := strings.SplitN(raw, "\t", 5)
	if len(fields) < 4 {
		return nil, fmt.Errorf("unexpected stat output: %q", raw)
	}
	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("stat size parse: %w", err)
	}
	// GNU stat `%f` is raw hex mode combining type bits + permission bits.
	rawMode, err := strconv.ParseUint(fields[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("stat mode parse: %w", err)
	}
	mtime, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("stat mtime parse: %w", err)
	}
	linkTarget := ""
	if len(fields) == 5 {
		// `%N` prints `'src' -> 'target'` for symlinks, `'name'` otherwise.
		if idx := strings.Index(fields[4], "->"); idx != -1 {
			linkTarget = strings.Trim(strings.TrimSpace(fields[4][idx+2:]), "'")
		}
	}

	name := fields[0]
	if name == "" {
		name = requestedPath
	}
	// Extract basename.
	if i := strings.LastIndex(name, "/"); i != -1 {
		name = name[i+1:]
	}
	return &api.ContainerPathStat{
		Name:       name,
		Size:       size,
		Mode:       unixModeToFileMode(uint32(rawMode)),
		Mtime:      time.Unix(mtime, 0).UTC(),
		LinkTarget: linkTarget,
	}, nil
}

// unixModeToFileMode converts a POSIX `st_mode` (raw hex from `stat
// -c %f`) into os.FileMode (Go's mode). Permission bits round-trip
// 1:1; the type bits map differently.
func unixModeToFileMode(m uint32) os.FileMode {
	mode := os.FileMode(m & 0o777)
	switch m & 0o170000 {
	case 0o040000:
		mode |= os.ModeDir
	case 0o120000:
		mode |= os.ModeSymlink
	case 0o140000:
		mode |= os.ModeSocket
	case 0o020000:
		mode |= os.ModeCharDevice | os.ModeDevice
	case 0o060000:
		mode |= os.ModeDevice
	case 0o010000:
		mode |= os.ModeNamedPipe
	}
	if m&0o004000 != 0 {
		mode |= os.ModeSetuid
	}
	if m&0o002000 != 0 {
		mode |= os.ModeSetgid
	}
	if m&0o001000 != 0 {
		mode |= os.ModeSticky
	}
	return mode
}
