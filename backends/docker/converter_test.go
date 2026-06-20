package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/sockerless/api"
)

// TestConvertContainerJSON_PopulatesState verifies that docker inspect
// surfaces the container's State block. The goverter directive that was
// meant to map State is not honored by the generated converter, so
// ConvertContainerJSON sets it explicitly; without that, every inspect
// would report an empty State (Status:"", Running:false, Pid:0, ...).
func TestConvertContainerJSON_PopulatesState(t *testing.T) {
	info := types.ContainerJSON{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   "abc123",
			Name: "/running-ctr",
			State: &container.State{
				Status:   "running",
				Running:  true,
				Pid:      4242,
				ExitCode: 0,
			},
		},
	}

	c := ConvertContainerJSON(info)

	if c.State.Status != "running" {
		t.Errorf("State.Status = %q, want %q", c.State.Status, "running")
	}
	if !c.State.Running {
		t.Errorf("State.Running = false, want true")
	}
	if c.State.Pid != 4242 {
		t.Errorf("State.Pid = %d, want 4242", c.State.Pid)
	}
}

// TestConvertContainerJSON_ExitedState verifies a non-zero exit code is
// carried through (CI gates and `docker wait`/inspect read ExitCode).
func TestConvertContainerJSON_ExitedState(t *testing.T) {
	info := types.ContainerJSON{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID: "dead1",
			State: &container.State{
				Status:   "exited",
				Running:  false,
				ExitCode: 137,
			},
		},
	}

	c := ConvertContainerJSON(info)
	if c.State.Status != "exited" {
		t.Errorf("State.Status = %q, want %q", c.State.Status, "exited")
	}
	if c.State.ExitCode != 137 {
		t.Errorf("State.ExitCode = %d, want 137", c.State.ExitCode)
	}
}

// TestEndpointSettings_DNSNamesAndLinks_RoundTrip verifies DNSNames and
// Links survive the read converter and both write converters, so
// --network-alias / --link reach the daemon and inspect reports them.
func TestEndpointSettings_DNSNamesAndLinks_RoundTrip(t *testing.T) {
	dnsNames := []string{"web", "web.mynet"}
	links := []string{"db:database"}

	// Read: docker SDK -> api.
	dockerEP := &network.EndpointSettings{
		NetworkID: "net1",
		Aliases:   []string{"web"},
		DNSNames:  dnsNames,
		Links:     links,
	}
	apiEP := EndpointSettingsToAPI(dockerEP)
	if got := apiEP.DNSNames; !equalStrs(got, dnsNames) {
		t.Errorf("EndpointSettingsToAPI DNSNames = %v, want %v", got, dnsNames)
	}
	if got := apiEP.Links; !equalStrs(got, links) {
		t.Errorf("EndpointSettingsToAPI Links = %v, want %v", got, links)
	}

	// Write (network connect): api -> docker SDK.
	back := APIEndpointToDocker(apiEP)
	if got := back.DNSNames; !equalStrs(got, dnsNames) {
		t.Errorf("APIEndpointToDocker DNSNames = %v, want %v", got, dnsNames)
	}
	if got := back.Links; !equalStrs(got, links) {
		t.Errorf("APIEndpointToDocker Links = %v, want %v", got, links)
	}

	// Write (docker run --network): api -> docker SDK.
	nc := mapNetworkingConfigToDocker(&api.NetworkingConfig{
		EndpointsConfig: map[string]*api.EndpointSettings{"mynet": apiEP},
	})
	es := nc.EndpointsConfig["mynet"]
	if got := es.DNSNames; !equalStrs(got, dnsNames) {
		t.Errorf("mapNetworkingConfigToDocker DNSNames = %v, want %v", got, dnsNames)
	}
	if got := es.Links; !equalStrs(got, links) {
		t.Errorf("mapNetworkingConfigToDocker Links = %v, want %v", got, links)
	}
}

// TestConvertContainerSummary_HostConfigNetworkMode verifies docker ps
// reports HostConfig.NetworkMode (used by `--filter network=`).
func TestConvertContainerSummary_HostConfigNetworkMode(t *testing.T) {
	c := types.Container{ID: "x"}
	c.HostConfig.NetworkMode = "host"

	summary := ConvertContainerSummary(c)
	if summary.HostConfig == nil {
		t.Fatalf("summary.HostConfig is nil")
	}
	if summary.HostConfig.NetworkMode != "host" {
		t.Errorf("HostConfig.NetworkMode = %q, want %q", summary.HostConfig.NetworkMode, "host")
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
