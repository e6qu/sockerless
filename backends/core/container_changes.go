package core

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// ChangeKind matches Docker's ContainerChangeKind enumeration:
// 0 = modified (file or dir existed in image, altered since)
// 1 = added    (file or dir created since container started)
// 2 = deleted  (file or dir removed — not detectable via find(1), omitted)
const (
	ChangeKindModified = 0
	ChangeKindAdded    = 1
	ChangeKindDeleted  = 2
)

// RunContainerChangesViaAgent implements `docker diff` by running
// `find / -xdev -newer /proc/1 -printf '%y\t%p\n'` inside the
// container over the reverse-agent. This surfaces every path that was
// created or modified after the init process started — covering
// Added + Modified. Detecting Deleted paths requires image-layer
// access that isn't available from inside the container; the sockerless
// reverse-agent path reports this as a known limitation rather than
// lying with a fabricated diff.
func RunContainerChangesViaAgent(reg *ReverseAgentRegistry, containerID string) ([]api.ContainerChangeItem, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	// -xdev      stay on the root filesystem (skip overlay mounts)
	// -newer     reference is /proc/1 — bootstrap PID 1 starts at
	//            container-boot so everything newer was touched during
	//            this invocation
	// -printf    one entry per line: type-char TAB path. `d`=dir,
	//            `f`=regular file, `l`=symlink, `b/c`=block/char, etc.
	argv := []string{"find", "/", "-xdev", "-newer", "/proc/1", "-printf", `%y` + "\t" + `%p` + "\n"}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, "diff-"+containerID, argv, nil, "")
	if err != nil {
		return nil, err
	}
	// find returns 1 when a path can't be read (permission denied, etc.)
	// which is common at / — those are non-fatal. 0 and 1 both yield a
	// usable diff; anything else is a real failure.
	if exit != 0 && exit != 1 {
		return nil, fmt.Errorf("find failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))
	}
	return ParseChangesOutput(string(stdout)), nil
}

// ParseChangesOutput converts find's `%y\t%p\n` rows into
// api.ContainerChangeItem entries. Every path is reported as
// Added (kind 1). Modified-in-place detection (mtime ≠ image mtime but
// path predates /proc/1) isn't reachable from inside the container
// without a side-channel to the image layer.
func ParseChangesOutput(raw string) []api.ContainerChangeItem {
	out := make([]api.ContainerChangeItem, 0)
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		path := fields[1]
		// Skip /proc and /sys noise that find sometimes emits before
		// -xdev takes effect on nested mount-namespaces.
		if strings.HasPrefix(path, "/proc/") || strings.HasPrefix(path, "/sys/") {
			continue
		}
		out = append(out, api.ContainerChangeItem{
			Kind: ChangeKindAdded,
			Path: path,
		})
	}
	return out
}
