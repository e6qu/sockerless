package docker

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// FuzzMapDockerError drives the Docker-error string/JSON classifier. It parses
// arbitrary daemon error strings (some are JSON, some are "No such X: Y"
// templates) and must never panic on a malformed message — including ones that
// look like a truncated "No such " or a not-found suffix with no preceding
// token.
func FuzzMapDockerError(f *testing.F) {
	seeds := []string{
		"",
		"No such container: abc",
		"No such ",
		"No such:",
		"No such image",
		" not found",
		"foo not found",
		"not found",
		"is already in use",
		"Conflict. The container name is in use",
		`{"message":"boom"}`,
		`{"message":""}`,
		`{"message":`,
		"not modified",
		"No such \x00: \x00",
		"a: b not found No such ",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, msg string) {
		_ = mapDockerError(errors.New(msg))
		_, _ = parseDockerNotFound(msg)
	})
}

// FuzzPortConverters drives the nat.PortSet / nat.PortMap converters with
// arbitrary `port/proto`-shaped (or malformed) keys. These build api maps from
// attacker-influenced Docker port keys and must never panic on a key missing a
// slash, an empty key, or a non-UTF-8 key.
func FuzzPortConverters(f *testing.F) {
	seeds := []string{"80/tcp", "", "/", "80", "65536/udp", "/tcp", "\x00", "a/b/c"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, portKey string) {
		ps := nat.PortSet{nat.Port(portKey): struct{}{}}
		_ = PortSetToMap(ps)
		_ = StringSetToMap(map[string]struct{}{portKey: {}})

		pm := nat.PortMap{nat.Port(portKey): []nat.PortBinding{{HostIP: portKey, HostPort: portKey}}}
		_ = PortMapToBindings(pm)

		// ConvertHostConfig consumes the port map + arbitrary string fields.
		hc := container.HostConfig{
			Binds:        []string{portKey},
			PortBindings: pm,
			Tmpfs:        map[string]string{portKey: portKey},
			Sysctls:      map[string]string{portKey: portKey},
		}
		_ = ConvertHostConfig(hc)
	})
}
