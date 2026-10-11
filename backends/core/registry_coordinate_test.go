package core

import "testing"

func TestRegistryCoordinate(t *testing.T) {
	for _, tc := range []struct{ in, host, url string }{
		{"", "", ""},
		{"  ", "", ""},
		{"http://127.0.0.1:4566/", "127.0.0.1:4566", "http://127.0.0.1:4566"},
		{"https://registry.example.com", "registry.example.com", "https://registry.example.com"},
		{"registry.example.com", "registry.example.com", "https://registry.example.com"},
	} {
		if got := RegistryCoordinateHost(tc.in); got != tc.host {
			t.Errorf("RegistryCoordinateHost(%q) = %q, want %q", tc.in, got, tc.host)
		}
		if got := RegistryCoordinateURL(tc.in); got != tc.url {
			t.Errorf("RegistryCoordinateURL(%q) = %q, want %q", tc.in, got, tc.url)
		}
	}
}
