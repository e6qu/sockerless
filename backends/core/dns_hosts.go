package core

import "strings"

// ResolvePeerHosts returns host:ip pairs for all peer containers that share
// a network with the given container. For each peer, entries are produced for:
//   - the container name (without leading "/")
//   - Config.Hostname (if set)
//   - all aliases from the shared network's EndpointSettings
func ResolvePeerHosts(store *Store, containerID string) []string {
	c, ok := store.Containers.Get(containerID)
	if !ok {
		return nil
	}

	// Find shared networks (skip built-in)
	var sharedNetworks []string
	for netName := range c.NetworkSettings.Networks {
		if netName == "bridge" || netName == "host" || netName == "none" || netName == "default" {
			continue
		}
		sharedNetworks = append(sharedNetworks, netName)
	}
	if len(sharedNetworks) == 0 {
		return nil
	}

	// Collect peer containers from the same pod or same network
	peerIDs := make(map[string]bool)
	if pod, inPod := store.Pods.GetPodForContainer(containerID); inPod {
		for _, cid := range pod.ContainerIDs {
			if cid != containerID {
				peerIDs[cid] = true
			}
		}
	}

	var hosts []string
	for peerID := range peerIDs {
		peer, ok := store.Containers.Get(peerID)
		if !ok {
			continue
		}

		// Find the peer's IP on a shared network
		var peerIP string
		var peerAliases []string
		for _, netName := range sharedNetworks {
			if ep := peer.NetworkSettings.Networks[netName]; ep != nil {
				if ep.IPAddress != "" {
					peerIP = ep.IPAddress
				}
				peerAliases = append(peerAliases, ep.Aliases...)
			}
		}
		if peerIP == "" {
			peerIP = "127.0.0.1" // fallback for pods sharing localhost
		}

		// Container name (trim leading "/")
		name := strings.TrimPrefix(peer.Name, "/")
		if name != "" {
			hosts = append(hosts, name+":"+peerIP)
		}

		// Hostname
		if peer.Config.Hostname != "" && peer.Config.Hostname != name {
			hosts = append(hosts, peer.Config.Hostname+":"+peerIP)
		}

		// Aliases
		for _, alias := range peerAliases {
			if alias != name && alias != peer.Config.Hostname {
				hosts = append(hosts, alias+":"+peerIP)
			}
		}
	}

	return hosts
}
