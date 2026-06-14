package core

import "testing"

func TestWorkloadArch(t *testing.T) {
	cases := map[string]string{
		"":        "amd64",
		"amd64":   "amd64",
		"x86_64":  "amd64",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"ARM64":   "arm64",
		"weird":   "amd64",
	}
	for in, want := range cases {
		t.Setenv("SOCKERLESS_WORKLOAD_ARCH", in)
		if got := workloadArch(); got != want {
			t.Errorf("workloadArch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectPlatformManifest(t *testing.T) {
	multiArch := []manifestListItem{
		{Digest: "sha256:amd", Platform: manifestPlatform{OS: "linux", Architecture: "amd64"}},
		{Digest: "sha256:arm", Platform: manifestPlatform{OS: "linux", Architecture: "arm64"}},
	}
	armOnly := []manifestListItem{
		{Digest: "sha256:arm", Platform: manifestPlatform{OS: "linux", Architecture: "arm64"}},
	}
	amdOnly := []manifestListItem{
		{Digest: "sha256:amd", Platform: manifestPlatform{OS: "linux", Architecture: "amd64"}},
	}

	// Multi-arch: each arch selects its own manifest.
	if d, _ := selectPlatformManifest(multiArch, "arm64"); d != "sha256:arm" {
		t.Errorf("multi-arch arm64 → %q, want sha256:arm", d)
	}
	if d, _ := selectPlatformManifest(multiArch, "amd64"); d != "sha256:amd" {
		t.Errorf("multi-arch amd64 → %q, want sha256:amd", d)
	}
	// Empty arch defaults to amd64.
	if d, _ := selectPlatformManifest(multiArch, ""); d != "sha256:amd" {
		t.Errorf("multi-arch default → %q, want sha256:amd", d)
	}
	// arm64-only (the gitlab-runner-helper `arm64-…` tag shape): an arm64
	// request selects it — the previous hardcoded-amd64 logic failed here.
	if d, _ := selectPlatformManifest(armOnly, "arm64"); d != "sha256:arm" {
		t.Errorf("arm64-only arm64 → %q, want sha256:arm", d)
	}
	// arm64 request against amd64-only falls back to amd64 rather than failing.
	if d, _ := selectPlatformManifest(amdOnly, "arm64"); d != "sha256:amd" {
		t.Errorf("amd64-only arm64 (fallback) → %q, want sha256:amd", d)
	}
	// Neither present → empty digest + available list for the error.
	armOnlyForAmd := selectAvail(t, armOnly)
	if armOnlyForAmd == "" {
		t.Errorf("expected available platforms listed")
	}
}

func selectAvail(t *testing.T, manifests []manifestListItem) string {
	t.Helper()
	// amd64 request where only a non-amd64, non-requested arch exists: here
	// arm64-only with an amd64 request DOES fall back? No — amd64 request on
	// arm64-only: pick("amd64")="" and wantArch==amd64 so no fallback → "".
	d, avail := selectPlatformManifest(manifests, "amd64")
	if d != "" {
		t.Errorf("amd64 on arm64-only → %q, want empty", d)
	}
	if len(avail) == 0 {
		return ""
	}
	return avail[0]
}
