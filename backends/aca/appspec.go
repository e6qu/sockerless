package aca

import (
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	core "github.com/sockerless/backend-core"
)

// Phase 88 — Apps spec builder. Parallel to jobspec.go. When
// Config.UseApp is true, container execution switches from ACA Jobs
// to ACA Apps with internal-only ingress so peers have stable
// per-revision FQDNs (BUG-716).

// buildAppName generates an ACA ContainerApp name from a container ID.
// Distinct prefix from buildJobName so Jobs and Apps never collide in
// the same resource group when UseApp is toggled across containers.
func buildAppName(containerID string) string {
	return fmt.Sprintf("sockerless-app-%s", containerID[:12])
}

// buildAppSpec creates an ACA ContainerApp resource from one or more
// containers. Internal ingress + min/max replicas = 1 keeps the app
// long-running with a peer-reachable internal FQDN. Callers must have
// Config.UseApp set; this builder does not enforce that.
func (s *Server) buildAppSpec(containers []containerInput) armappcontainers.ContainerApp {
	var specs []*armappcontainers.Container
	for _, ci := range containers {
		specs = append(specs, s.buildContainerSpec(ci))
	}

	environmentID := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
		s.config.SubscriptionID, s.config.ResourceGroup, s.config.Environment,
	)

	mainContainer := containers[0].Container
	networkName := "bridge"
	if mainContainer.HostConfig.NetworkMode != "" && mainContainer.HostConfig.NetworkMode != "default" {
		networkName = mainContainer.HostConfig.NetworkMode
	}

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "aca",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		Name:        mainContainer.Name,
		Network:     networkName,
		Labels:      mainContainer.Config.Labels,
	}
	// Carry pod membership through to App tags so ListPods can
	// reconstruct docker pods after a restart.
	if pod, _ := s.Store.Pods.GetPodForContainer(containers[0].ID); pod != nil {
		tags.Pod = pod.Name
	}

	ingress := &armappcontainers.Ingress{
		External:   ptr(false),
		TargetPort: ptr(int32(8080)),
		Transport:  ptr(armappcontainers.IngressTransportMethodAuto),
	}

	activeRevMode := armappcontainers.ActiveRevisionsModeSingle
	minR, maxR := int32(1), int32(1)

	return armappcontainers.ContainerApp{
		Location: ptr(s.config.Location),
		Tags:     tags.AsAzurePtrMap(),
		Properties: &armappcontainers.ContainerAppProperties{
			EnvironmentID: ptr(environmentID),
			Configuration: &armappcontainers.Configuration{
				ActiveRevisionsMode: &activeRevMode,
				Ingress:             ingress,
			},
			Template: &armappcontainers.Template{
				Containers: specs,
				Scale: &armappcontainers.Scale{
					MinReplicas: &minR,
					MaxReplicas: &maxR,
				},
			},
		},
	}
}
