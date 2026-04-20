package cloudrun

import "testing"

// TestServiceURIHost_StripsScheme — canonical Cloud Run Service.Uri
// includes scheme; Cloud DNS CNAME targets want just the hostname.
func TestServiceURIHost_StripsScheme(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://sockerless-svc-abc-xxx.a.run.app", "sockerless-svc-abc-xxx.a.run.app"},
		{"http://svc-internal.example.com/", "svc-internal.example.com"},
		{"", ""},
		// Already hostname-only: strip trailing slash, keep the rest.
		{"plain.host.example/", "plain.host.example"},
	}
	for _, tc := range cases {
		got := serviceURIHost(tc.in)
		if got != tc.want {
			t.Errorf("serviceURIHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
