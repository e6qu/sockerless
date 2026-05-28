package main

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/network"
	sim "github.com/sockerless/simulator"
)

func workloadCallbackHost() string {
	info := strings.ToLower(sim.RuntimeInfo())
	if strings.Contains(info, "podman") {
		if gateway := defaultWorkloadNetworkGateway(); gateway != "" {
			return gateway
		}
	}
	return "host.docker.internal"
}

func defaultWorkloadNetworkGateway() string {
	cli := sim.DockerClient()
	if cli == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, name := range []string{"podman", "bridge"} {
		networkInfo, err := cli.NetworkInspect(ctx, name, network.InspectOptions{})
		if err != nil {
			continue
		}
		for _, cfg := range networkInfo.IPAM.Config {
			if cfg.Gateway != "" {
				return cfg.Gateway
			}
		}
	}
	return ""
}
