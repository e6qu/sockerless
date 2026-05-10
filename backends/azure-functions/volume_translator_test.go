package azf

import (
	"strings"
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestTranslateBackingSpecMemoryRejected(t *testing.T) {
	// Azure Functions WebApps storage surface is BYOS-only; no
	// tmpfs primitive exists at the AzureStorageInfoValue layer.
	// Per-invocation /tmp is the closest analogue but not a
	// Docker-style mount. Translator rejects loudly. Phase 91b.
	spec := core.BackingSpec{
		Kind:   core.BackingMemory,
		Memory: &core.MemorySpec{SizeMB: 64},
	}
	_, err := translateBackingSpecToAZFStorage(spec, "/mnt/ws", "secret")
	if err == nil {
		t.Fatal("expected error for BackingMemory on AZF")
	}
	msg := err.Error()
	for _, want := range []string{"memory", "/tmp"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}
