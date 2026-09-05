package cloudrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
	"google.golang.org/protobuf/types/known/durationpb"
)

// containerInput groups the data needed to build one Cloud Run container spec.
type containerInput struct {
	ID        string
	Container *api.Container
	IsMain    bool // true = primary container in a pod
}

// buildJobName generates a Cloud Run Job name from a container ID.
func buildJobName(containerID string) string {
	return fmt.Sprintf("sockerless-%s", containerID[:12])
}

// buildJobParent returns the Cloud Run parent resource path.
func (s *Server) buildJobParent() string {
	return fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
}

// buildContainerSpec builds a single Cloud Run container spec plus
// any `VolumeMount` entries its Docker `HostConfig.Binds` produce.
// Host-path binds are already rejected at `ContainerCreate` so every
// entry here is a `volName:/mnt[:ro]` pair.
func (s *Server) buildContainerSpec(ci containerInput) (*runpb.Container, []*runpb.VolumeMount) {
	config := ci.Container.Config

	// Build environment variables
	var envVars []*runpb.EnvVar
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   parts[0],
				Values: &runpb.EnvVar_Value{Value: parts[1]},
			})
		}
	}

	// Carry Docker labels as the single authoritative SOCKERLESS_LABELS env
	// var (core.LabelsEnvVar) on the main container; cloud_state.go reads it
	// back into Container.Config.Labels.
	if ci.IsMain {
		if v, ok := core.EncodeLabelsEnvValue(config.Labels); ok {
			envVars = append(envVars, &runpb.EnvVar{
				Name:   core.LabelsEnvVar,
				Values: &runpb.EnvVar_Value{Value: v},
			})
		}
	}

	// Inject reverse-agent callback URL + container ID so the bootstrap
	// baked into the container image can dial back for `docker exec` /
	// `docker attach`. CallbackURL is required at NewServer time so by
	// here it is guaranteed non-empty (no fallback for missing-agent).
	if ci.IsMain {
		envVars = append(envVars,
			&runpb.EnvVar{Name: "SOCKERLESS_CALLBACK_URL", Values: &runpb.EnvVar_Value{Value: s.config.CallbackURL}},
			&runpb.EnvVar{Name: "SOCKERLESS_CONTAINER_ID", Values: &runpb.EnvVar_Value{Value: ci.ID}},
		)
		// Workload-facing storage coordinate for the bootstrap's gcs-sync
		// workspace restore/save (standard STORAGE_EMULATOR_HOST). Empty on
		// real Cloud Run (bootstrap uses real GCS + ADC); a sim harness sets
		// it to a workload-reachable sim storage address.
		if s.config.GCSWorkloadEndpoint != "" {
			envVars = append(envVars,
				&runpb.EnvVar{Name: "STORAGE_EMULATOR_HOST", Values: &runpb.EnvVar_Value{Value: s.config.GCSWorkloadEndpoint}},
			)
		}
	}

	entrypoint := config.Entrypoint
	command := config.Cmd

	// Container name
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	cpu, memory := mapCPUMemory(ci)

	var mounts []*runpb.VolumeMount
	var syncMountEntries []string
	for _, bind := range ci.Container.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		mounts = append(mounts, &runpb.VolumeMount{
			Name:      parts[0],
			MountPath: parts[1],
		})
		// Record the bind target for gcs-sync SharedVolumes so the
		// bootstrap restore knows where to untar each per-exec GCS
		// object. PreExec emits just `name=GCS_URL` (the runner-task
		// can't know the bind target on its own); the bootstrap joins
		// by name. Bind sources are pre-translated to SharedVolume.Name
		// at ContainerCreate time — see cloudrun/backend_impl.go's
		// overlay-bind translator. Look up by name here.
		if sv := s.config.SharedVolumes.ByName(parts[0]); sv != nil && sv.Backing == core.BackingGCSSync {
			syncMountEntries = append(syncMountEntries, fmt.Sprintf("%s=%s", sv.Name, parts[1]))
		}
	}
	if ci.IsMain && len(syncMountEntries) > 0 {
		envVars = append(envVars, &runpb.EnvVar{
			Name:   "SOCKERLESS_SYNC_MOUNTS",
			Values: &runpb.EnvVar_Value{Value: strings.Join(syncMountEntries, ",")},
		})
	}

	containerSpec := &runpb.Container{
		Name:         defName,
		Image:        config.Image,
		Command:      entrypoint,
		Args:         command,
		Env:          envVars,
		VolumeMounts: mounts,
		Resources: &runpb.ResourceRequirements{
			Limits: map[string]string{
				"cpu":    cpu,
				"memory": memory,
			},
		},
	}

	// Cloud Run Service health check probes ContainerPort. Read the
	// actual ExposedPorts from the image (real cloud-primitive data, not
	// a hardcoded heuristic). If the image declares no ports, the
	// container does NOT bind $PORT and is NOT eligible for Service path —
	// let the caller route it elsewhere or fail loudly. No defaults, no
	// fallbacks (per project rule).
	//
	// Cloud Run multi-container rule: EXACTLY ONE container per revision
	// must declare Ports — the ingress one. The bootstrap (which is the
	// main container's PID 1 in overlay images) listens on the value of
	// the standard Cloud Run PORT env (default 8080), regardless of what
	// the image's Config.ExposedPorts declares. Force-declare 8080 on
	// the main container so multi-container revisions are accepted.
	// Sidecars must omit Ports entirely AND set SOCKERLESS_SIDECAR=1 so
	// their bootstrap exec's the user CMD as a foreground subprocess
	// instead of trying to bind PORT (which would conflict with main's
	// bind).
	if ci.IsMain {
		port := imagePort(ci.Container)
		if port == 0 {
			port = 8080
		}
		containerSpec.Ports = []*runpb.ContainerPort{
			{ContainerPort: int32(port)},
		}
	} else {
		containerSpec.Env = append(containerSpec.Env, &runpb.EnvVar{
			Name:   "SOCKERLESS_SIDECAR",
			Values: &runpb.EnvVar_Value{Value: "1"},
		})
	}

	// Deliberately DO NOT set containerSpec.WorkingDir. Cloud Run
	// validates the WorkingDir exists on the container's mount fs at
	// startup — under gcs-sync the workspace mount is an empty tmpfs
	// (bootstrap restores from GCS per-exec), so a workdir like
	// /__w/sockerless/sockerless wouldn't exist yet and Cloud Run
	// would fail with "Application failed to start: failed to find
	// initial working directory". The bootstrap chdir's per-exec via
	// envelope.Workdir and per-default-invoke via SOCKERLESS_USER_WORKDIR
	// env, so the user's workdir is honoured at the right scope.

	return containerSpec, mounts
}

// imagePort returns the first port the image declares via
// Config.ExposedPorts. Reads the real image metadata; no hardcoded
// per-image port maps. Returns 0 if image declares none.
func imagePort(c *api.Container) int {
	if c == nil {
		return 0
	}
	for portKey := range c.Config.ExposedPorts {
		var port int
		_, _ = fmt.Sscanf(portKey, "%d", &port)
		if port > 0 {
			return port
		}
	}
	return 0
}

// buildJobSpec creates a Cloud Run Job protobuf from one or more
// containers, provisioning a GCS bucket per referenced named volume
// and injecting matching RevisionTemplate Volumes.
func (s *Server) buildJobSpec(ctx context.Context, containers []containerInput) (*runpb.Job, error) {
	var specs []*runpb.Container
	volSeen := make(map[string]struct{})
	var volumes []*runpb.Volume
	var persistEntries []string
	for _, ci := range containers {
		cs, mounts := s.buildContainerSpec(ci)
		specs = append(specs, cs)
		for _, mp := range mounts {
			if _, done := volSeen[mp.Name]; done {
				continue
			}
			vol, persist, err := s.volumes.VolumeForBind(ctx, mp.Name, mp.MountPath)
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, vol)
			if persist != "" {
				persistEntries = append(persistEntries, persist)
			}
			volSeen[mp.Name] = struct{}{}
		}
	}
	gcpcommon.InjectPersistEnv(specs, persistEntries)

	// Cloud-side cap on top of the bootstrap-side timer. Both layers
	// share the same intent value (core.JobTimeoutDefault). Cloud Run
	// Jobs cap at 24 h regardless of requested value.
	timeoutSec := core.JobTimeoutDefault()
	if timeoutSec <= 0 || timeoutSec > 24*3600 {
		timeoutSec = 24 * 3600
	}
	taskTemplate := &runpb.TaskTemplate{
		Containers: specs,
		Volumes:    volumes,
		Retries:    &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
		Timeout:    durationpb.New(time.Duration(timeoutSec) * time.Second),
	}

	// Add VPC connector if configured
	if s.config.VPCConnector != "" {
		// ALL_TRAFFIC — see servicespec.go for the full rationale
		// (Cloud NAT in the connector subnet keeps public APIs reachable;
		// in-VPC source needed for Cloud Run service-to-service Ingress=
		// internal acceptance).
		taskTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_ALL_TRAFFIC,
		}
	}

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "cloudrun",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		Name:        containers[0].Container.Name,
		Network:     containers[0].Container.HostConfig.NetworkMode,
		AutoRemove:  containers[0].Container.HostConfig.AutoRemove,
	}
	// Propagate pod membership so ListPods can reconstruct docker pods
	// from the cloud's Job labels after a backend restart.
	if pod, _ := s.Store.Pods.GetPodForContainer(containers[0].ID); pod != nil {
		tags.Pod = pod.Name
	}

	return &runpb.Job{
		Labels:      tags.AsGCPLabels(),
		Annotations: tags.AsGCPAnnotations(),
		Template: &runpb.ExecutionTemplate{
			TaskCount:   1,
			Parallelism: 1,
			Template:    taskTemplate,
		},
	}, nil
}

// cloudRunCombo is a valid Cloud Run (gen2) per-container CPU/memory
// pairing. memMinMiB/memMaxMiB are the documented min/max memory for a
// container declaring that many vCPU. Cloud Run accepts any integer-MiB
// value in the range, so a request snaps to its own size clamped to the
// tier's bounds rather than a fixed step.
type cloudRunCombo struct {
	cpu       int   // 1, 2, 4, 8
	memMinMiB int64 // documented minimum memory for this vCPU count
	memMaxMiB int64 // documented maximum memory for this vCPU count
}

// cloudRunCombos lists the valid Cloud Run vCPU tiers with their memory
// envelopes (per the Cloud Run resource-limits reference). Higher vCPU
// counts raise both the minimum and maximum allowed memory.
var cloudRunCombos = []cloudRunCombo{
	{1, 512, 4096},
	{2, 512, 8192},
	{4, 2048, 16384},
	{8, 4096, 32768},
}

// mapCPUMemory derives a Cloud Run per-container CPU/memory limit from the
// container's real Docker resource request, snapping to the smallest valid
// vCPU tier that satisfies both the requested CPU and memory. With no
// request it returns the historical default floor (1 vCPU / 4Gi) — chosen
// to leave headroom for the in-Service bootstrap (~256 MiB), the 2 GiB
// tmpfs default, and a postgres-style sidecar.
func mapCPUMemory(ci containerInput) (string, string) {
	hc := ci.Container.HostConfig

	var reqMemMiB int64
	if hc.Memory > 0 {
		reqMemMiB = hc.Memory / (1024 * 1024)
	} else if hc.MemoryReservation > 0 {
		reqMemMiB = hc.MemoryReservation / (1024 * 1024)
	}

	var reqCPU int64 // in whole vCPU (rounded up)
	if hc.NanoCPUs > 0 {
		reqCPU = (hc.NanoCPUs + 1e9 - 1) / 1e9
	} else if hc.CPUShares > 0 {
		// Docker CPUShares are relative weights with 1024 == 1 vCPU.
		reqCPU = (hc.CPUShares + 1023) / 1024
	}

	if reqMemMiB <= 0 && reqCPU <= 0 {
		// Zero-request floor/default.
		return "1", "4Gi"
	}

	for _, combo := range cloudRunCombos {
		if int64(combo.cpu) < reqCPU {
			continue
		}
		if reqMemMiB > combo.memMaxMiB {
			continue
		}
		mem := reqMemMiB
		if mem < combo.memMinMiB {
			mem = combo.memMinMiB
		}
		return fmt.Sprintf("%d", combo.cpu), fmt.Sprintf("%dMi", mem)
	}

	// Request exceeds the largest tier — clamp to the maximum Cloud Run
	// offers rather than under-provisioning.
	last := cloudRunCombos[len(cloudRunCombos)-1]
	return fmt.Sprintf("%d", last.cpu), fmt.Sprintf("%dMi", last.memMaxMiB)
}

// sanitizeContainerName converts a container name to a valid Cloud Run
// container name per RFC 1123: lowercase ASCII letters/digits/hyphens
// and periods, must begin and end with letter or digit, length < 64.
func sanitizeContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.':
			b.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			b.WriteRune(c + 32) // lowercase
		default:
			b.WriteByte('-')
		}
	}
	result := b.String()
	// Trim leading non-alphanumeric (must begin with letter or digit).
	for len(result) > 0 && !isAlnum(result[0]) {
		result = result[1:]
	}
	// Cap to 50 chars (leave room for any future suffixes; Cloud Run
	// limit is 63).
	if len(result) > 50 {
		// Keep a stable hash of the original to avoid collisions when
		// multiple long names share the same 50-char prefix.
		hash := nameHash(name)
		result = result[:50-9] + "-" + hash
	}
	// Trim trailing non-alphanumeric (must end with letter or digit).
	for len(result) > 0 && !isAlnum(result[len(result)-1]) {
		result = result[:len(result)-1]
	}
	if result == "" {
		return "sidecar"
	}
	return result
}

func isAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// nameHash returns a short 8-char hex hash for use as a name disambiguator.
func nameHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
