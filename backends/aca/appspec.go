package aca

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	core "github.com/sockerless/backend-core"
)

// — Apps spec builder. Parallel to jobspec.go. When
// Config.UseApp is true, container execution switches from ACA Jobs
// to ACA Apps with internal-only ingress so peers have stable
// per-revision FQDNs.

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
func (s *Server) buildAppSpec(ctx context.Context, containers []containerInput) (armappcontainers.ContainerApp, error) {
	var specs []*armappcontainers.Container
	volSeen := make(map[string]struct{})
	var volumes []*armappcontainers.Volume
	storageType := armappcontainers.StorageTypeAzureFile
	for _, ci := range containers {
		cs, mounts := s.buildContainerSpec(ci)
		specs = append(specs, cs)
		for _, mp := range mounts {
			if mp.VolumeName == nil {
				continue
			}
			volName := *mp.VolumeName
			if _, done := volSeen[volName]; done {
				continue
			}
			share, err := s.shareForVolume(ctx, volName)
			if err != nil {
				return armappcontainers.ContainerApp{}, fmt.Errorf("provision Azure Files share for volume %q: %w", volName, err)
			}
			volumes = append(volumes, &armappcontainers.Volume{
				Name:        ptr(volName),
				StorageType: &storageType,
				StorageName: ptr(share),
			})
			volSeen[volName] = struct{}{}
		}
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
				Volumes:    volumes,
				Scale: &armappcontainers.Scale{
					MinReplicas: &minR,
					MaxReplicas: &maxR,
				},
			},
		},
	}, nil
}
