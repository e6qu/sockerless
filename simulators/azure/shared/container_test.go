package simulator

import "testing"

// parsePlatform — workload arch is carried in the spec, never derived
// from the host. Empty input → nil (Docker uses image default).
func TestParsePlatform(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" if expecting nil
	}{
		{"", ""},
		{"linux/arm64", "linux/arm64"},
		{"linux/amd64", "linux/amd64"},
		{"linux/arm/v7", "linux/arm/v7"},
		{"garbage", ""},
	}
	for _, tc := range cases {
		got := parsePlatform(tc.in)
		if tc.want == "" {
			if got != nil {
				t.Errorf("parsePlatform(%q) = %+v, want nil", tc.in, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("parsePlatform(%q) = nil, want %s", tc.in, tc.want)
			continue
		}
		flat := got.OS + "/" + got.Architecture
		if got.Variant != "" {
			flat += "/" + got.Variant
		}
		if flat != tc.want {
			t.Errorf("parsePlatform(%q) = %s, want %s", tc.in, flat, tc.want)
		}
	}
}
