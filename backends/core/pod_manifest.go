package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// A multi-container pod on a FaaS backend is materialised as one function
// whose image carries every member's root filesystem under
// /containers/<name>, with the sockerless bootstrap as PID 1 supervising
// the members. The manifest below is how the backend tells the bootstrap
// what to run, and how the backend later reconstructs each member's
// `docker ps` row from the function alone: the manifest travels in the
// function's SOCKERLESS_POD_CONTAINERS environment variable, so no local
// state is needed.

// PodMemberSpec is one container inside a pod overlay as the backend
// knows it at build time.
type PodMemberSpec struct {
	// Name is the container's name inside the pod: its chroot subdir and
	// its supervisor log prefix.
	Name string
	// ContainerID is the sockerless container ID the member represents.
	ContainerID string
	// BaseImageRef is the member's image, resolved to a pullable reference.
	BaseImageRef string
	// Entrypoint, Cmd, Workdir, and Env are the member's command, with the
	// image's defaults already merged in by the backend.
	Entrypoint []string
	Cmd        []string
	Workdir    string
	Env        []string
}

// PodMemberJSON is the wire shape of one member in SOCKERLESS_POD_CONTAINERS.
// ContainerID and Image are read back by the backend's cloud state; the
// bootstrap ignores them.
type PodMemberJSON struct {
	Name        string   `json:"name"`
	Root        string   `json:"root"`
	Entrypoint  []string `json:"entrypoint,omitempty"`
	Cmd         []string `json:"cmd,omitempty"`
	Env         []string `json:"env,omitempty"`
	Workdir     string   `json:"workdir,omitempty"`
	ContainerID string   `json:"container_id,omitempty"`
	Image       string   `json:"image,omitempty"`
}

// EncodePodManifest returns the base64(JSON) value of
// SOCKERLESS_POD_CONTAINERS. Each member's Root is its merged-rootfs
// subdirectory, `/containers/<name>`.
func EncodePodManifest(members []PodMemberSpec) (string, error) {
	out := make([]PodMemberJSON, len(members))
	for i, m := range members {
		out[i] = PodMemberJSON{
			Name:        m.Name,
			Root:        "/containers/" + m.Name,
			Entrypoint:  m.Entrypoint,
			Cmd:         m.Cmd,
			Env:         m.Env,
			Workdir:     m.Workdir,
			ContainerID: m.ContainerID,
			Image:       m.BaseImageRef,
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DecodePodManifest inverts EncodePodManifest. An empty value decodes to
// no members.
func DecodePodManifest(b64 string) ([]PodMemberJSON, error) {
	if b64 == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	var out []PodMemberJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return out, nil
}

// SanitizePodMemberName reduces a container name to the character set a
// cloud resource name and a chroot subdirectory both accept: lower-case
// letters, digits, and hyphens, with `_` and `.` folded to `-`. A name
// with nothing left becomes "x".
func SanitizePodMemberName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	return out
}

// SanitizePodLabelValue reduces a value to the lower-case letters, digits,
// hyphens, and underscores that AWS tag values and Google Cloud label
// values both accept.
func SanitizePodLabelValue(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}
