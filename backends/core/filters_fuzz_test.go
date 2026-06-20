package core

import (
	"testing"

	"github.com/sockerless/api"
)

// FuzzParseFilters drives the Docker filter-JSON parser with malformed,
// nested, and adversarial JSON. ParseFilters must never panic — on any
// undecodable input it returns nil.
func FuzzParseFilters(f *testing.F) {
	seeds := []string{
		"",
		"{}",
		`{"label":{"a=b":true}}`,
		`{"label":["a=b"]}`,
		`{"status":{"running":true},"name":{"foo":true}}`,
		`{"label":{"":true}}`,
		`{"label":{"=":true}}`,
		`{"label":{"k":false}}`,
		`{"":{"":true}}`,
		`[1,2,3]`,
		`null`,
		`"a string"`,
		`12345`,
		`{"a":`,
		`{"a":{"b":}}`,
		`{"label":{"` + string([]byte{0xff, 0xfe}) + `":true}}`,
		`{"a":{"b":{"c":true}}}`,
		`{"deeply":` + deepNest(2000) + `}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := ParseFilters(s)
		// Feed whatever it produced straight into the consumers — they must
		// also tolerate any key/value the parser emits.
		_ = MatchContainerFilters(api.Container{}, got)
		_ = MatchNetworkFilters(api.Network{}, got)
		_ = MatchNetworkPruneFilters(api.Network{}, got)
		_ = MatchVolumeFilters(api.Volume{}, got)
	})
}

func deepNest(n int) string {
	open := make([]byte, 0, n*7)
	for i := 0; i < n; i++ {
		open = append(open, []byte(`{"x":`)...)
	}
	open = append(open, '1')
	for i := 0; i < n; i++ {
		open = append(open, '}')
	}
	return string(open)
}

// FuzzMatchContainerFilters drives the container filter matcher with arbitrary
// filter keys/values directly (bypassing the JSON parser) to hit the
// per-key match branches (status/label/exited/publish/...).
func FuzzMatchContainerFilters(f *testing.F) {
	f.Add("label", "a=b")
	f.Add("label", "")
	f.Add("label", "=")
	f.Add("exited", "")
	f.Add("exited", "99999999999999999999999999")
	f.Add("exited", "-1")
	f.Add("publish", "")
	f.Add("publish", "/")
	f.Add("status", "running")
	f.Add("name", "")
	f.Add("expose", "")
	f.Add("is-task", "true")
	f.Fuzz(func(t *testing.T, key, val string) {
		filters := map[string][]string{key: {val, ""}}
		_ = MatchContainerFilters(api.Container{
			Config:     api.ContainerConfig{Labels: map[string]string{"a": "b"}},
			HostConfig: api.HostConfig{PortBindings: map[string][]api.PortBinding{"80/tcp": nil}},
		}, filters)
	})
}
