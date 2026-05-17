package core

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateTmpfsFitsMemory returns an error when the tmpfs request
// plus a fixed 256 MiB headroom (kernel + bootstrap + app baseline)
// would exceed the configured container memory. Used by cloudrun /
// ACA at materialize-time and by GCF at startup. Fail loud per the
// no-fallback rule — caller surfaces the error rather than clamping.
// containerMiB <= 0 means "memory unspecified by operator"; the
// check is skipped and the caller is expected to either set the
// memory or accept whatever the cloud's default is.
func ValidateTmpfsFitsMemory(tmpfsMiB, containerMiB int, backend, containerID string) error {
	const headroomMiB = 256
	if containerMiB <= 0 {
		return nil
	}
	if tmpfsMiB+headroomMiB > containerMiB {
		return fmt.Errorf(
			"%s container %s: tmpfs cap %d MiB + %d MiB headroom exceeds container memory %d MiB — "+
				"raise the container memory request, lower SOCKERLESS_%s_TMPFS_SIZE_MIB, or switch to a persistent storage backing",
			backend, containerID, tmpfsMiB, headroomMiB, containerMiB,
			strings.ToUpper(backend),
		)
	}
	return nil
}

// ParseMemoryMiB converts a Kubernetes / Cloud Run / GCF style memory
// string ("512Mi", "1Gi", "2Gi", "1024M", "1G", or bare integer
// "1024" interpreted as MiB) into integer MiB. Used by backends to
// validate tmpfs cap against function-memory at startup or
// materialize-time. Mismatch fails loud (no clamping).
//
// Both Kubernetes-style ("Mi", "Gi", binary base) and legacy
// ("M", "G", decimal base) suffixes are accepted; both are treated
// as binary because the difference is negligible at the scales we
// care about and the operator-facing error message stays simpler.
func ParseMemoryMiB(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("memory string is empty")
	}
	digits := s
	mult := 1
	switch {
	case strings.HasSuffix(s, "Mi"):
		digits = strings.TrimSuffix(s, "Mi")
		mult = 1
	case strings.HasSuffix(s, "Gi"):
		digits = strings.TrimSuffix(s, "Gi")
		mult = 1024
	case strings.HasSuffix(s, "M"):
		digits = strings.TrimSuffix(s, "M")
		mult = 1
	case strings.HasSuffix(s, "G"):
		digits = strings.TrimSuffix(s, "G")
		mult = 1024
	}
	n, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("memory value must be positive (got %d)", n)
	}
	return n * mult, nil
}
