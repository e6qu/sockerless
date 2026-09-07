package docker

import (
	"fmt"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/image"
	"net"
	"strings"
	"time"

	dockerocispec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/sockerless/api"
)

// dockerOCIToContainerConfig narrows v28's DockerOCIImageConfig (the
// new shape returned by ImageInspect.Config) into the older
// container.Config shape that ConvertContainerConfig expects. The two
// types share semantically-equivalent fields; this helper copies them
// straight across.
func dockerOCIToContainerConfig(c dockerocispec.DockerOCIImageConfig) container.Config {
	cfg := container.Config{
		User:        c.User,
		Env:         c.Env,
		Entrypoint:  c.Entrypoint,
		Cmd:         c.Cmd,
		WorkingDir:  c.WorkingDir,
		Labels:      c.Labels,
		StopSignal:  c.StopSignal,
		ArgsEscaped: c.ArgsEscaped,
		OnBuild:     c.OnBuild,
		Shell:       c.Shell,
		Healthcheck: c.Healthcheck,
		Volumes:     c.Volumes,
	}
	return cfg
}

// conv is the singleton generated converter instance.
// Initialized via initConverter() in converter_init.go (build tag !goverter).
var conv Converter

// ConvertContainerJSON converts a full Docker ContainerJSON (inspect response)
// to our api.Container, composing generated sub-converters with manual handling
// for HostConfig (embedded Resources struct) and NetworkSettings (embedded bases).
func ConvertContainerJSON(info container.InspectResponse) api.Container {
	c := conv.ConvertContainerBase(info)

	// The generated ConvertContainerBase does not populate State (the
	// goverter `map . State` directive is not honored by the generated
	// code), so map it explicitly here. Without this, `docker inspect`
	// reports an empty State block (Status:"", Running:false, Pid:0,
	// ExitCode:0, Health:nil) for every container.
	c.State = MapContainerState(info)

	if info.Config != nil {
		c.Config = conv.ConvertContainerConfig(*info.Config)
		c.Config.ExposedPorts = PortSetToMap(info.Config.ExposedPorts)
		if info.Config.Healthcheck != nil {
			hc := conv.ConvertHealthcheckConfig(*info.Config.Healthcheck)
			c.Config.Healthcheck = &hc
		}
	}

	if info.HostConfig != nil {
		c.HostConfig = ConvertHostConfig(*info.HostConfig)
	}

	c.NetworkSettings = ConvertNetworkSettings(info.NetworkSettings)
	c.Mounts = MountPointsToAPI(info.Mounts)

	return c
}

// ConvertHostConfig converts a Docker container.HostConfig to api.HostConfig.
// This is manual because Docker's HostConfig embeds Resources (a non-pointer struct),
// which goverter doesn't flatten into our api.HostConfig's flat fields.
func ConvertHostConfig(hc container.HostConfig) api.HostConfig {
	result := api.HostConfig{
		NetworkMode:       string(hc.NetworkMode),
		Binds:             hc.Binds,
		AutoRemove:        hc.AutoRemove,
		PortBindings:      PortMapToBindings(hc.PortBindings),
		RestartPolicy:     conv.ConvertRestartPolicy(hc.RestartPolicy),
		Privileged:        hc.Privileged,
		CapAdd:            hc.CapAdd,
		CapDrop:           hc.CapDrop,
		Init:              hc.Init,
		UsernsMode:        string(hc.UsernsMode),
		ShmSize:           hc.ShmSize,
		Tmpfs:             hc.Tmpfs,
		SecurityOpt:       hc.SecurityOpt,
		LogConfig:         LogConfigToAPI(hc.LogConfig),
		ExtraHosts:        hc.ExtraHosts,
		Mounts:            DockerMountsToAPI(hc.Mounts),
		Isolation:         string(hc.Isolation),
		DNS:               AddrsToStrings(hc.DNS),
		DNSSearch:         hc.DNSSearch,
		DNSOptions:        hc.DNSOptions,
		Memory:            hc.Memory,
		MemorySwap:        hc.MemorySwap,
		MemoryReservation: hc.MemoryReservation,
		CPUShares:         hc.CPUShares,
		CPUQuota:          hc.CPUQuota,
		CPUPeriod:         hc.CPUPeriod,
		CpusetCpus:        hc.CpusetCpus,
		NanoCPUs:          hc.NanoCPUs,
		CpusetMems:        hc.CpusetMems,
		BlkioWeight:       hc.BlkioWeight,
		PidMode:           string(hc.PidMode),
		IpcMode:           string(hc.IpcMode),
		UTSMode:           string(hc.UTSMode),
		VolumesFrom:       hc.VolumesFrom,
		GroupAdd:          hc.GroupAdd,
		ReadonlyRootfs:    hc.ReadonlyRootfs,
		OomKillDisable:    hc.OomKillDisable,
		PidsLimit:         hc.PidsLimit,
		Sysctls:           hc.Sysctls,
		Runtime:           hc.Runtime,
		Links:             hc.Links,
		PublishAllPorts:   hc.PublishAllPorts,
		CgroupnsMode:      string(hc.CgroupnsMode),
	}
	if hc.ConsoleSize != [2]uint{} {
		cs := hc.ConsoleSize
		result.ConsoleSize = &cs
	}
	return result
}

// ConvertNetworkSettings converts Docker's NetworkSettings (with embedded bases) to api.NetworkSettings.
func ConvertNetworkSettings(ns *container.NetworkSettings) api.NetworkSettings {
	result := api.NetworkSettings{
		Networks: make(map[string]*api.EndpointSettings),
	}
	if ns == nil {
		return result
	}
	result.SandboxID = ns.SandboxID
	result.SandboxKey = ns.SandboxKey
	result.Ports = PortMapToBindings(ns.Ports)

	// The Docker API's container-wide address fields describe the default
	// bridge endpoint, the same values its Networks entry carries; the
	// daemon stopped serving the separate copies, so they are derived from
	// that endpoint here.
	if bridge := ns.Networks["bridge"]; bridge != nil {
		result.Gateway = AddrToString(bridge.Gateway)
		result.IPAddress = AddrToString(bridge.IPAddress)
		result.IPPrefixLen = bridge.IPPrefixLen
		result.MacAddress = HardwareAddrToString(bridge.MacAddress)
		result.EndpointID = bridge.EndpointID
		result.IPv6Gateway = AddrToString(bridge.IPv6Gateway)
		result.GlobalIPv6Address = AddrToString(bridge.GlobalIPv6Address)
		result.GlobalIPv6PrefixLen = bridge.GlobalIPv6PrefixLen
	}

	// Networks map
	result.Networks = EndpointSettingsMapToAPI(ns.Networks)
	if result.Networks == nil {
		result.Networks = make(map[string]*api.EndpointSettings)
	}

	return result
}

// ConvertContainerSummary converts a Docker container list entry to api.ContainerSummary.
func ConvertContainerSummary(c container.Summary) *api.ContainerSummary {
	summary := &api.ContainerSummary{
		ID:         c.ID,
		Names:      c.Names,
		Image:      c.Image,
		ImageID:    c.ImageID,
		Command:    c.Command,
		Created:    c.Created,
		State:      string(c.State),
		Status:     c.Status,
		Labels:     c.Labels,
		SizeRw:     c.SizeRw,
		SizeRootFs: c.SizeRootFs,
		Mounts:     MountPointsToAPI(c.Mounts),
		HostConfig: &api.HostConfigSummary{NetworkMode: c.HostConfig.NetworkMode},
	}
	for _, p := range c.Ports {
		summary.Ports = append(summary.Ports, conv.ConvertPort(p))
	}
	if c.NetworkSettings != nil && len(c.NetworkSettings.Networks) > 0 {
		summary.NetworkSettings = &api.SummaryNetworkSettings{
			Networks: EndpointSettingsMapToAPI(c.NetworkSettings.Networks),
		}
	}
	return summary
}

// ConvertImageInspect converts a Docker ImageInspect to api.Image.
func ConvertImageInspect(info image.InspectResponse) api.Image {
	img := conv.ConvertImageBase(info)
	// The API dropped VirtualSize; clients that still read it expect the
	// image's size.
	img.VirtualSize = info.Size

	if info.Config != nil {
		img.Config = conv.ConvertContainerConfig(dockerOCIToContainerConfig(*info.Config))
		img.Config.ExposedPorts = StringSetToMap(info.Config.ExposedPorts)
		if info.Config.Healthcheck != nil {
			hc := conv.ConvertHealthcheckConfig(*info.Config.Healthcheck)
			img.Config.Healthcheck = &hc
		}
	}

	if info.RootFS.Type != "" {
		img.RootFS = api.RootFS{
			Type:   info.RootFS.Type,
			Layers: info.RootFS.Layers,
		}
	}

	if !info.Metadata.LastTagTime.IsZero() {
		img.Metadata.LastTagTime = info.Metadata.LastTagTime.Format(time.RFC3339Nano)
	}

	return img
}

// ConvertNetworkResource converts a Docker network inspect result to api.Network.
func ConvertNetworkResource(n network.Inspect) api.Network {
	net := api.Network{
		Name:       n.Name,
		ID:         n.ID,
		Created:    n.Created.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Scope:      n.Scope,
		Driver:     n.Driver,
		EnableIPv6: n.EnableIPv6,
		Internal:   n.Internal,
		Attachable: n.Attachable,
		Ingress:    n.Ingress,
		Labels:     n.Labels,
		Options:    n.Options,
	}
	ConvertNetworkIPAMAndContainers(&net, n.IPAM, n.Containers)
	return net
}

// ConvertNetworkSummary converts a Docker network list entry (network.Summary)
// to api.Network.
func ConvertNetworkSummary(n network.Summary) api.Network {
	net := api.Network{
		Name:       n.Name,
		ID:         n.ID,
		Created:    n.Created.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Scope:      n.Scope,
		Driver:     n.Driver,
		EnableIPv6: n.EnableIPv6,
		Internal:   n.Internal,
		Attachable: n.Attachable,
		Ingress:    n.Ingress,
		Labels:     n.Labels,
		Options:    n.Options,
	}
	// A network listing carries no endpoint map; inspect does.
	ConvertNetworkIPAMAndContainers(&net, n.IPAM, nil)
	return net
}

// ConvertNetworkIPAMAndContainers populates IPAM and Containers on a network.
func ConvertNetworkIPAMAndContainers(net *api.Network, ipam network.IPAM, containers map[string]network.EndpointResource) {
	if ipam.Driver != "" || len(ipam.Config) > 0 {
		configs := make([]api.IPAMConfig, 0, len(ipam.Config))
		for _, c := range ipam.Config {
			configs = append(configs, conv.ConvertIPAMConfig(c))
		}
		net.IPAM = api.IPAM{
			Driver:  ipam.Driver,
			Config:  configs,
			Options: ipam.Options,
		}
	}
	if len(containers) > 0 {
		net.Containers = make(map[string]api.EndpointResource, len(containers))
		for id, ep := range containers {
			net.Containers[id] = conv.ConvertEndpointResource(ep)
		}
	}
}

// ConvertVolume converts a Docker volume to api.Volume.
func ConvertVolumeSDK(v volume.Volume) api.Volume {
	return conv.ConvertVolume(v)
}

// APIEndpointToDocker converts an api.EndpointSettings to Docker's network.EndpointSettings.
func APIEndpointToDocker(ep *api.EndpointSettings) (*network.EndpointSettings, error) {
	if ep == nil {
		return nil, nil
	}
	gateway, err := parseAddr("gateway", ep.Gateway)
	if err != nil {
		return nil, err
	}
	ipAddress, err := parseAddr("IP address", ep.IPAddress)
	if err != nil {
		return nil, err
	}
	ipv6Gateway, err := parseAddr("IPv6 gateway", ep.IPv6Gateway)
	if err != nil {
		return nil, err
	}
	globalIPv6, err := parseAddr("global IPv6 address", ep.GlobalIPv6Address)
	if err != nil {
		return nil, err
	}
	var mac network.HardwareAddr
	if ep.MacAddress != "" {
		parsed, err := net.ParseMAC(ep.MacAddress)
		if err != nil {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid MAC address %q: %v", ep.MacAddress, err)}
		}
		mac = network.HardwareAddr(parsed)
	}
	es := &network.EndpointSettings{
		NetworkID:           ep.NetworkID,
		EndpointID:          ep.EndpointID,
		Gateway:             gateway,
		IPAddress:           ipAddress,
		IPPrefixLen:         ep.IPPrefixLen,
		IPv6Gateway:         ipv6Gateway,
		GlobalIPv6Address:   globalIPv6,
		GlobalIPv6PrefixLen: ep.GlobalIPv6PrefixLen,
		MacAddress:          mac,
		Aliases:             ep.Aliases,
		DNSNames:            ep.DNSNames,
		Links:               ep.Links,
		DriverOpts:          ep.DriverOpts,
	}
	if ep.IPAMConfig != nil {
		ipv4, err := parseAddr("IPAM IPv4 address", ep.IPAMConfig.IPv4Address)
		if err != nil {
			return nil, err
		}
		ipv6, err := parseAddr("IPAM IPv6 address", ep.IPAMConfig.IPv6Address)
		if err != nil {
			return nil, err
		}
		linkLocal, err := parseAddrs("link-local address", ep.IPAMConfig.LinkLocalIPs)
		if err != nil {
			return nil, err
		}
		es.IPAMConfig = &network.EndpointIPAMConfig{
			IPv4Address:  ipv4,
			IPv6Address:  ipv6,
			LinkLocalIPs: linkLocal,
		}
	}
	return es, nil
}

// ConvertBuildCache converts Docker build cache entries to api.BuildCache.
func ConvertBuildCache(bc build.CacheRecord) api.BuildCache {
	return api.BuildCache{
		ID:          bc.ID,
		Parent:      strings.Join(bc.Parents, ","),
		Type:        bc.Type,
		Description: bc.Description,
		InUse:       bc.InUse,
		Shared:      bc.Shared,
		Size:        bc.Size,
		CreatedAt:   bc.CreatedAt.Format(time.RFC3339Nano),
		LastUsedAt:  bc.LastUsedAt.Format(time.RFC3339Nano),
		UsageCount:  bc.UsageCount,
	}
}
