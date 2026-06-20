package core

import "testing"

// FuzzParseMemoryMiB drives the memory-string parser with arbitrary input. It
// must never panic; it either returns a positive value or an error. It must
// also never return a non-positive value with a nil error.
func FuzzParseMemoryMiB(f *testing.F) {
	for _, s := range []string{"", " ", "0", "-1", "512Mi", "1Gi", "1G", "512M", "  64  ", "Mi", "Gi", "99999999999999999999Gi", "0x10", "1.5Gi", "+5Mi", "9223372036854775807G", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := ParseMemoryMiB(s)
		if err == nil && v <= 0 {
			t.Fatalf("ParseMemoryMiB(%q) returned non-positive %d with nil error", s, v)
		}
	})
}

// FuzzParseImageRef drives the image-reference parser with arbitrary input. It
// must never panic and, on success, never return an empty Path.
func FuzzParseImageRef(f *testing.F) {
	for _, s := range []string{"", "alpine", "alpine:latest", "registry:5000/repo/image:tag", "image@sha256:" + repeatN("a", 64), "a/b/c/d:tag@sha256:bad", "::", "/", "@", ":", "localhost:5000/", "host:port:tag", "\x00\xff", repeatN("a/", 1000) + "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ref, err := ParseImageRef(s)
		if err == nil && ref.Path == "" {
			t.Fatalf("ParseImageRef(%q) returned empty Path with nil error", s)
		}
	})
}

// FuzzParseDockerTimestamp drives the since/until timestamp parser with
// arbitrary input. It must never panic.
func FuzzParseDockerTimestamp(f *testing.F) {
	for _, s := range []string{"", "0", "1700000000", "1700000000.5", "2023-01-01T00:00:00Z", "-1", "9999999999999999999999", "1e308", "NaN", "Inf", "+Inf", "0x1", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseDockerTimestamp(s)
	})
}

func repeatN(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
