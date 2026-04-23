package aca

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// containerInput groups the data needed to build one ACA container spec.
type containerInput struct {
	ID        string
	Container *api.Container
	IsMain    bool // true = primary container in a pod
}

// buildJobName generates an ACA Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildContainerSpec builds a single ACA container spec plus any
// `VolumeMount` entries its Docker `HostConfig.Binds` produce. Host
// binds are already rejected at ContainerCreate so every bind here
// is `volName:/mnt[:ro]`.
func (s *Server) buildContainerSpec(ci containerInput) (*armappcontainers.Container, []*armappcontainers.VolumeMount) {
	config := ci.Container.Config

	// Build environment variables
	var envVars []*armappcontainers.EnvironmentVar
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, &armappcontainers.EnvironmentVar{
				Name:  ptr(parts[0]),
				Value: ptr(parts[1]),
			})
		}
	}

	// Phase 96: inject reverse-agent callback URL + container ID so a
	// bootstrap baked into the container image can dial back for
	// `docker exec` / `docker attach`. Empty CallbackURL ⇒ reverse-agent
	// disabled (exec returns 126 for this container).
	if ci.IsMain && s.config.CallbackURL != "" {
		envVars = append(envVars,
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CALLBACK_URL"), Value: ptr(s.config.CallbackURL)},
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(ci.ID)},
		)
	}

	entrypoint := config.Entrypoint
	cmdArgs := config.Cmd

	// Convert entrypoint and command to []*string
	var command []*string
	for _, arg := range entrypoint {
		command = append(command, ptr(arg))
	}
	var args []*string
	for _, arg := range cmdArgs {
		args = append(args, ptr(arg))
	}

	// Container name
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	cpu, memory := mapCPUTier()

	var mounts []*armappcontainers.VolumeMount
	for _, bind := range ci.Container.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		mounts = append(mounts, &armappcontainers.VolumeMount{
			VolumeName: ptr(parts[0]),
			MountPath:  ptr(parts[1]),
		})
	}

	return &armappcontainers.Container{
		Name:         ptr(defName),
		Image:        ptr(config.Image),
		Command:      command,
		Args:         args,
		Env:          envVars,
		VolumeMounts: mounts,
		Resources: &armappcontainers.ContainerResources{
			CPU:    &cpu,
			Memory: ptr(memory),
		},
	}, mounts
}

// buildJobSpec creates an ACA Job resource from one or more containers,
// provisioning an Azure Files share + env-storage per referenced named
// volume and injecting matching JobTemplate Volumes.
func (s *Server) buildJobSpec(ctx context.Context, containers []containerInput) (armappcontainers.Job, error) {
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
				return armappcontainers.Job{}, fmt.Errorf("provision Azure Files share for volume %q: %w", volName, err)
			}
			volumes = append(volumes, &armappcontainers.Volume{
				Name:        ptr(volName),
				StorageType: &storageType,
				StorageName: ptr(share),
			})
			volSeen[volName] = struct{}{}
		}
	}

	replicaTimeout := int32(3600 * 4) // 4 hours max

	environmentID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
		s.config.SubscriptionID, s.config.ResourceGroup, s.config.Environment)

	triggerType := armappcontainers.TriggerTypeManual

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
	// Propagate pod membership so ListPods can reconstruct docker pods
	// from Job tags after a backend restart.
	if pod, _ := s.Store.Pods.GetPodForContainer(containers[0].ID); pod != nil {
		tags.Pod = pod.Name
	}

	return armappcontainers.Job{
		Location: ptr(s.config.Location),
		Tags:     tags.AsAzurePtrMap(),
		Properties: &armappcontainers.JobProperties{
			EnvironmentID: ptr(environmentID),
			Configuration: &armappcontainers.JobConfiguration{
				TriggerType:    &triggerType,
				ReplicaTimeout: &replicaTimeout,
				ManualTriggerConfig: &armappcontainers.JobConfigurationManualTriggerConfig{
					Parallelism:            ptr(int32(1)),
					ReplicaCompletionCount: ptr(int32(1)),
				},
				ReplicaRetryLimit: ptr(int32(0)),
			},
			Template: &armappcontainers.JobTemplate{
				Containers: specs,
				Volumes:    volumes,
			},
		},
	}, nil
}

// mapCPUTier returns the default ACA CPU/memory tier.
// Valid ACA CPU tiers: 0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 4.0.
// Default: 1.0 CPU, 2Gi.
func mapCPUTier() (float64, string) {
	return 1.0, "2Gi"
}

func ptr[T any](v T) *T {
	return &v
}

// sanitizeContainerName converts a container name to a valid ACA container name.
// Strips leading "/" and replaces non-alphanumeric characters with "-". Lowercased.
func sanitizeContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		} else if c >= 'A' && c <= 'Z' {
			b.WriteRune(c + 32) // lowercase
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()
	if result == "" {
		return "sidecar"
	}
	return result
}
