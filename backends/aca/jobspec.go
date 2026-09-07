package aca

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
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

	// Inject reverse-agent callback URL + container ID so the bootstrap
	// baked into the container image can dial back for `docker exec` /
	// `docker attach`. CallbackURL is required at NewServer time so
	// it's guaranteed non-empty here — no fallback for missing-agent.
	if ci.IsMain {
		envVars = append(envVars,
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CALLBACK_URL"), Value: ptr(s.config.CallbackURL)},
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(ci.ID)},
		)
	} else {
		envVars = append(envVars,
			&armappcontainers.EnvironmentVar{Name: ptr("SOCKERLESS_SIDECAR"), Value: ptr("1")},
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

	cpu, memory := mapCPUTier(ci)

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
			vol, err := s.resolveVolumeForName(ctx, volName)
			if err != nil {
				return armappcontainers.Job{}, err
			}
			volumes = append(volumes, vol)
			volSeen[volName] = struct{}{}
		}
	}

	// Cloud-side ACA cap on top of any bootstrap timer. ACA's
	// ReplicaTimeout caps at 7 days (604800s); we honour the shared
	// sockerless intent (default 3600 = 1h) and clamp.
	replicaTimeout := int32(core.JobTimeoutDefault())
	if replicaTimeout <= 0 || replicaTimeout > 604800 {
		replicaTimeout = 604800
	}

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

	// ExposedPorts have no ACA template field — carry the image's declared
	// ports through the Job tags so docker inspect/ps reflect them (mirrors the
	// App path in appspec.go).
	azureTags := tags.AsAzurePtrMap()
	if ports := encodeExposedPorts(mainContainer.Config.ExposedPorts); ports != "" {
		azureTags[tagExposedPorts] = ptr(ports)
	}

	return armappcontainers.Job{
		Location: ptr(s.config.Location),
		Tags:     azureTags,
		Identity: s.workloadIdentity(),
		Properties: &armappcontainers.JobProperties{
			EnvironmentID: ptr(environmentID),
			Configuration: &armappcontainers.JobConfiguration{
				TriggerType:    &triggerType,
				ReplicaTimeout: &replicaTimeout,
				Registries:     s.registryCredentials(),
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

// acaCombo is a valid ACA (Consumption workload profile) CPU/memory
// pairing. ACA enforces a fixed memory:cpu ratio of 2:1 (Gi:vCPU), so each
// tier's memory is exactly 2× its CPU.
type acaCombo struct {
	cpu   float64
	memGi float64
}

// acaCombos lists the valid ACA Consumption CPU/memory combinations in
// ascending order. CPU steps by 0.25 up to 2.0, then a 4.0/8Gi tier; the
// paired memory is always 2× CPU (the documented Consumption ratio).
var acaCombos = []acaCombo{
	{0.25, 0.5},
	{0.5, 1.0},
	{0.75, 1.5},
	{1.0, 2.0},
	{1.25, 2.5},
	{1.5, 3.0},
	{1.75, 3.5},
	{2.0, 4.0},
	{4.0, 8.0},
}

// mapCPUTier derives an ACA CPU/memory tier from the container's real
// Docker resource request, snapping to the smallest valid combo that
// satisfies both the requested CPU and memory (memory:cpu is pinned 2:1
// by ACA, so a memory-heavy request pulls CPU up). With no request it
// returns the historical default floor (2.0 vCPU / 4Gi) — leaving headroom
// for the in-container bootstrap (~256 MiB) plus the 2 GiB tmpfs default.
func mapCPUTier(ci containerInput) (float64, string) {
	hc := ci.Container.HostConfig

	var reqMemGi float64
	if hc.Memory > 0 {
		reqMemGi = float64(hc.Memory) / (1024 * 1024 * 1024)
	} else if hc.MemoryReservation > 0 {
		reqMemGi = float64(hc.MemoryReservation) / (1024 * 1024 * 1024)
	}

	var reqCPU float64
	if hc.NanoCPUs > 0 {
		reqCPU = float64(hc.NanoCPUs) / 1e9
	} else if hc.CPUShares > 0 {
		reqCPU = float64(hc.CPUShares) / 1024.0
	}

	if reqMemGi <= 0 && reqCPU <= 0 {
		// Zero-request floor/default.
		return 2.0, "4Gi"
	}

	for _, combo := range acaCombos {
		if combo.cpu+1e-9 < reqCPU {
			continue
		}
		if combo.memGi+1e-9 < reqMemGi {
			continue
		}
		return combo.cpu, formatGi(combo.memGi)
	}

	// Request exceeds the largest tier — clamp to the maximum ACA offers.
	last := acaCombos[len(acaCombos)-1]
	return last.cpu, formatGi(last.memGi)
}

// formatGi renders a Gi memory value as the ACA-expected string ("4Gi",
// "1.5Gi", "0.5Gi"), trimming a trailing ".0".
func formatGi(gi float64) string {
	s := strconv.FormatFloat(gi, 'f', -1, 64)
	return s + "Gi"
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
