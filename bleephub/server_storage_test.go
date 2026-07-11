package bleephub

import (
	"strings"
	"testing"
)

func TestPersistentServerStorageRequiresDurableGitAndObjectBytes(t *testing.T) {
	t.Setenv("BLEEPHUB_GIT_DIR", "")
	t.Setenv("BLEEPHUB_S3_BUCKET", "")
	if err := validatePersistentServerStorage(true); err == nil {
		t.Fatal("expected missing durable git storage to fail")
	} else if !strings.Contains(err.Error(), "git storage is in-memory") {
		t.Fatalf("git storage error = %v", err)
	}

	t.Setenv("BLEEPHUB_GIT_DIR", t.TempDir())
	if err := validatePersistentServerStorage(false); err == nil {
		t.Fatal("expected missing object byte storage to fail")
	} else if !strings.Contains(err.Error(), "service byte storage is not object-backed") ||
		!strings.Contains(err.Error(), "release assets") ||
		!strings.Contains(err.Error(), "container-registry blobs") ||
		!strings.Contains(err.Error(), "CodeQL database archives") ||
		!strings.Contains(err.Error(), "CodeQL variant-analysis query packs") ||
		!strings.Contains(err.Error(), "artifact attestation bundles") {
		t.Fatalf("object byte storage error = %v", err)
	}

	if err := validatePersistentServerStorage(true); err != nil {
		t.Fatalf("valid durable storage rejected: %v", err)
	}
}
