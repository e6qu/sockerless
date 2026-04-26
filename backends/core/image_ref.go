package core

import (
	"fmt"
	"strings"
)

// ImageRef is the canonical parsed form of a docker / OCI image
// reference. The shape mirrors the upstream `distribution/reference`
// breakdown: a registry domain, a path within that registry, and an
// optional tag or digest.
//
// String form: `<domain>/<path>[:<tag>][@<digest>]`. When the original
// reference omitted a domain, `Domain` is empty (the canonical default
// `docker.io` is *not* substituted — preserving original-vs-canonical
// is the caller's job; the parser stays mechanical).
//
// Replaces the ad-hoc `<name>:<tag>` splitting that used to happen at
// every callsite. Use `ParseImageRef` to construct, `String()` to
// serialize, the field accessors to inspect.
type ImageRef struct {
	Domain string // e.g. "ghcr.io", "public.ecr.aws", or "" for unqualified refs
	Path   string // repo path, e.g. "library/alpine"
	Tag    string // optional, mutually exclusive with Digest in canonical form
	Digest string // optional, e.g. "sha256:abc..."
}

// ParseImageRef breaks a reference string into its components. Empty
// inputs and malformed strings return an error; the parser does *not*
// invent default tags or domains. Callers that want `:latest` for
// untagged refs should add it themselves after parsing.
//
// Recognised forms:
//   - `name`                          → Path=name
//   - `name:tag`                      → Path=name, Tag=tag
//   - `name@sha256:...`               → Path=name, Digest=sha256:...
//   - `name:tag@sha256:...`           → all three
//   - `host/path`, `host:port/path`   → Domain split on first `/`
//   - `host/path:tag`, etc.           → as above plus tag/digest
//
// The "host has a colon" / "tag has a colon" disambiguation matches
// docker's: a colon to the left of any `/` is part of the registry
// host (host:port), a colon to the right of every `/` is the tag
// separator.
func ParseImageRef(s string) (ImageRef, error) {
	if s == "" {
		return ImageRef{}, fmt.Errorf("empty image reference")
	}
	rest := s

	// Split off digest first; it always comes last.
	var digest string
	if i := strings.Index(rest, "@"); i >= 0 {
		digest = rest[i+1:]
		rest = rest[:i]
		if digest == "" {
			return ImageRef{}, fmt.Errorf("empty digest in %q", s)
		}
	}

	// Domain split: the first segment is a domain only if it contains
	// a `.` or `:` or is exactly "localhost". Otherwise the whole
	// rest-before-tag is a path.
	var domain, path string
	if i := strings.Index(rest, "/"); i >= 0 {
		first := rest[:i]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			domain = first
			rest = rest[i+1:]
		}
	}

	// Tag is what's after the LAST `:` in `rest` (after domain
	// stripping). The earlier `host:port` ambiguity is resolved by
	// the domain split above.
	var tag string
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		tag = rest[i+1:]
		rest = rest[:i]
		if tag == "" {
			return ImageRef{}, fmt.Errorf("empty tag in %q", s)
		}
	}
	path = rest
	if path == "" {
		return ImageRef{}, fmt.Errorf("empty path in %q", s)
	}

	return ImageRef{Domain: domain, Path: path, Tag: tag, Digest: digest}, nil
}

// String returns the canonical reference form. Only the components
// that were set get serialized — `Path` alone produces just the path.
func (r ImageRef) String() string {
	var b strings.Builder
	if r.Domain != "" {
		b.WriteString(r.Domain)
		b.WriteByte('/')
	}
	b.WriteString(r.Path)
	if r.Tag != "" {
		b.WriteByte(':')
		b.WriteString(r.Tag)
	}
	if r.Digest != "" {
		b.WriteByte('@')
		b.WriteString(r.Digest)
	}
	return b.String()
}

// NameTag returns the `path:tag` slice the docker SDK splits an image
// reference into for ECR PutImage and similar APIs that take name +
// tag separately. The domain is not included; callers needing the
// fully-qualified ref should use String().
func (r ImageRef) NameTag() (name, tag string) {
	return r.Path, r.Tag
}

// FullName returns `<domain>/<path>` (or just `<path>` when there's no
// domain). Callers like ECR push that need a name without the tag use
// this.
func (r ImageRef) FullName() string {
	if r.Domain == "" {
		return r.Path
	}
	return r.Domain + "/" + r.Path
}
