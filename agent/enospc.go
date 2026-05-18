package agent

import (
	"bytes"
	"fmt"
)

// ENOSPCExitCode is curl(1)'s exit code 28 ("Operation timeout"). We
// repurpose it here for ENOSPC because Docker exec / kernel signals do
// not have a canonical exit code for "filesystem full" — 28 is widely
// recognised as "disk full" in tooling (Postfix's "queue file: ENOSPC"
// also maps to 28) and the operator-facing message disambiguates.
const ENOSPCExitCode = 28

// enospcMarkers — case-insensitive substrings the kernel and common
// runtimes emit on ENOSPC. Detected in subprocess stderr by the
// FaaS bootstraps so they can return a typed exit code + operator
// guidance instead of generic "exec failed exit 1."
var enospcMarkers = [][]byte{
	[]byte("No space left on device"),
	[]byte("no space left on device"),
	[]byte("ENOSPC"),
	[]byte("disk quota exceeded"),
}

// DetectENOSPC returns true when the stderr buffer contains any
// well-known ENOSPC marker. Cheap byte-substring scan; called once
// per exec result.
func DetectENOSPC(stderr []byte) bool {
	for _, m := range enospcMarkers {
		if bytes.Contains(stderr, m) {
			return true
		}
	}
	return false
}

// AnnotateENOSPC prepends a clear operator-guidance line to the
// stderr buffer when DetectENOSPC matched. The annotation cites the
// per-backend env var so the operator can act without reading the
// bootstrap source.
func AnnotateENOSPC(stderr []byte, backend string) []byte {
	hint := fmt.Sprintf(
		"sockerless: ENOSPC detected — tmpfs (or scratch volume) is full on the %s FaaS pod. "+
			"Raise SOCKERLESS_%s_TMPFS_SIZE_MIB (and the per-container memory request to fit) "+
			"or switch the SharedVolume Backing to a persistent driver (gcs-sync / pd-ephemeral / "+
			"efs-ephemeral / azure-files-ephemeral) for this workload.\n",
		backend, upperBackend(backend),
	)
	out := make([]byte, 0, len(hint)+len(stderr))
	out = append(out, []byte(hint)...)
	out = append(out, stderr...)
	return out
}

func upperBackend(b string) string {
	out := make([]byte, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		out[i] = c
	}
	return string(out)
}
