package docker

import (
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/sockerless/api"
)

// conv is the singleton generated converter instance.
// Initialized via initConverter() in converter_init.go (build tag !goverter).
var conv Converter

// ConvertContainerJSON converts a full Docker ContainerJSON (inspect response)
// to our api.Container, composing generated sub-converters with manual handling
// for HostConfig (embedded Resources struct) and NetworkSettings (embedded bases).
func ConvertContainerJSON(info types.ContainerJSON) api.Container {
	if info.ContainerJSONBase == nil {
		return api.Container{}
	}
	c := conv.ConvertContainerBase(*info.ContainerJSONBase)

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
		CapAdd:            []string(hc.CapAdd),
		CapDrop:           []string(hc.CapDrop),
		Init:              hc.Init,
		UsernsMode:        string(hc.UsernsMode),
		ShmSize:           hc.ShmSize,
		Tmpfs:             hc.Tmpfs,
		SecurityOpt:       hc.SecurityOpt,
		LogConfig:         LogConfigToAPI(hc.LogConfig),
		ExtraHosts:        hc.ExtraHosts,
		Mounts:            DockerMountsToAPI(hc.Mounts),
		Isolation:         string(hc.Isolation),
		DNS:               hc.DNS,
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
		ConsoleSize:       hc.ConsoleSize,
	}
	return result
}

// ConvertNetworkSettings converts Docker's NetworkSettings (with embedded bases) to api.NetworkSettings.
func ConvertNetworkSettings(ns *types.NetworkSettings) api.NetworkSettings {
	result := api.NetworkSettings{
		Networks: make(map[string]*api.EndpointSettings),
	}
	if ns == nil {
		return result
	}
	// From NetworkSettingsBase (embedded)
	result.Bridge = ns.Bridge
	result.SandboxID = ns.SandboxID
	result.SandboxKey = ns.SandboxKey
	result.HairpinMode = ns.HairpinMode
	result.Ports = PortMapToBindings(ns.Ports)

	// From DefaultNetworkSettings (embedded)
	result.Gateway = ns.Gateway
	result.IPAddress = ns.IPAddress
	result.IPPrefixLen = ns.IPPrefixLen
	result.MacAddress = ns.MacAddress
	result.EndpointID = ns.EndpointID
	result.IPv6Gateway = ns.IPv6Gateway
	result.GlobalIPv6Address = ns.GlobalIPv6Address
	result.GlobalIPv6PrefixLen = ns.GlobalIPv6PrefixLen
	result.LinkLocalIPv6Address = ns.LinkLocalIPv6Address
	result.LinkLocalIPv6PrefixLen = ns.LinkLocalIPv6PrefixLen

	// Networks map
	result.Networks = EndpointSettingsMapToAPI(ns.Networks)
	if result.Networks == nil {
		result.Networks = make(map[string]*api.EndpointSettings)
	}

	return result
}

// ConvertContainerSummary converts a Docker container list entry to api.ContainerSummary.
func ConvertContainerSummary(c types.Container) *api.ContainerSummary {
	summary := &api.ContainerSummary{
		ID:         c.ID,
		Names:      c.Names,
		Image:      c.Image,
		ImageID:    c.ImageID,
		Command:    c.Command,
		Created:    c.Created,
		State:      c.State,
		Status:     c.Status,
		Labels:     c.Labels,
		SizeRw:     c.SizeRw,
		SizeRootFs: c.SizeRootFs,
		Mounts:     MountPointsToAPI(c.Mounts),
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
func ConvertImageInspect(info types.ImageInspect) api.Image {
	img := conv.ConvertImageBase(info)

	if info.Config != nil {
		img.Config = conv.ConvertContainerConfig(*info.Config)
		img.Config.ExposedPorts = PortSetToMap(info.Config.ExposedPorts)
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

// ConvertNetworkSummary converts a Docker network list entry to api.Network.
func ConvertNetworkSummary(n types.NetworkResource) api.Network {
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
func APIEndpointToDocker(ep *api.EndpointSettings) *network.EndpointSettings {
	if ep == nil {
		return nil
	}
	es := &network.EndpointSettings{
		NetworkID:           ep.NetworkID,
		EndpointID:          ep.EndpointID,
		Gateway:             ep.Gateway,
		IPAddress:           ep.IPAddress,
		IPPrefixLen:         ep.IPPrefixLen,
		IPv6Gateway:         ep.IPv6Gateway,
		GlobalIPv6Address:   ep.GlobalIPv6Address,
		GlobalIPv6PrefixLen: ep.GlobalIPv6PrefixLen,
		MacAddress:          ep.MacAddress,
		Aliases:             ep.Aliases,
		DriverOpts:          ep.DriverOpts,
	}
	if ep.IPAMConfig != nil {
		es.IPAMConfig = &network.EndpointIPAMConfig{
			IPv4Address:  ep.IPAMConfig.IPv4Address,
			IPv6Address:  ep.IPAMConfig.IPv6Address,
			LinkLocalIPs: ep.IPAMConfig.LinkLocalIPs,
		}
	}
	return es
}

// ConvertBuildCache converts Docker build cache entries to api.BuildCache.
func ConvertBuildCache(bc types.BuildCache) api.BuildCache {
	return api.BuildCache{
		ID:          bc.ID,
		Parent:      bc.Parent,
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
