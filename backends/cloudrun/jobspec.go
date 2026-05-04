package cloudrun

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	// Carry Docker labels as a base64-JSON env var so they round-trip
	// through CloudState even if the GCP control-plane strips
	// annotations (e.g. unsupported storage path or sim behaviour).
	// cloud_state.go reads this variable back into Container.Config.Labels.
	if ci.IsMain && len(config.Labels) > 0 {
		labelsJSON, _ := json.Marshal(config.Labels)
		envVars = append(envVars, &runpb.EnvVar{
			Name:   "SOCKERLESS_LABELS",
			Values: &runpb.EnvVar_Value{Value: base64.StdEncoding.EncodeToString(labelsJSON)},
		})
	}

	// Inject reverse-agent callback URL + container ID so a bootstrap
	// baked into the container image can dial back for `docker exec` /
	// `docker attach`. Empty CallbackURL ⇒ reverse-agent disabled
	// (exec returns 126 for this container).
	if ci.IsMain && s.config.CallbackURL != "" {
		envVars = append(envVars,
			&runpb.EnvVar{Name: "SOCKERLESS_CALLBACK_URL", Values: &runpb.EnvVar_Value{Value: s.config.CallbackURL}},
			&runpb.EnvVar{Name: "SOCKERLESS_CONTAINER_ID", Values: &runpb.EnvVar_Value{Value: ci.ID}},
		)
	}

	entrypoint := config.Entrypoint
	command := config.Cmd

	// Container name
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	cpu, memory := mapCPUMemory()

	var mounts []*runpb.VolumeMount
	for _, bind := range ci.Container.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		mounts = append(mounts, &runpb.VolumeMount{
			Name:      parts[0],
			MountPath: parts[1],
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

	// Phase 122f: Cloud Run Service health check probes ContainerPort.
	// Read the actual ExposedPorts from the image (real cloud-primitive
	// data, not a hardcoded heuristic). If the image declares no ports,
	// the container does NOT bind $PORT and is NOT eligible for Service
	// path — let the caller route it elsewhere or fail loudly. No
	// defaults, no fallbacks (per project rule).
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

	if config.WorkingDir != "" {
		containerSpec.WorkingDir = config.WorkingDir
	}

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
	for _, ci := range containers {
		cs, mounts := s.buildContainerSpec(ci)
		specs = append(specs, cs)
		for _, mp := range mounts {
			if _, done := volSeen[mp.Name]; done {
				continue
			}
			bucket, err := s.bucketForVolume(ctx, mp.Name)
			if err != nil {
				return nil, fmt.Errorf("provision GCS bucket for volume %q: %w", mp.Name, err)
			}
			volumes = append(volumes, &runpb.Volume{
				Name: mp.Name,
				VolumeType: &runpb.Volume_Gcs{
					Gcs: &runpb.GCSVolumeSource{
						Bucket:       bucket,
						MountOptions: gcpcommon.RunnerWorkspaceMountOptions(),
					},
				},
			})
			volSeen[mp.Name] = struct{}{}
		}
	}

	taskTemplate := &runpb.TaskTemplate{
		Containers: specs,
		Volumes:    volumes,
		Retries:    &runpb.TaskTemplate_MaxRetries{MaxRetries: 0},
		Timeout:    durationpb.New(4 * time.Hour),
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

// mapCPUMemory returns the default Cloud Run resource limits.
// Cloud Run valid CPU: 1, 2, 4, 8. Default: 1 CPU, 512Mi.
func mapCPUMemory() (string, string) {
	return "1", "512Mi"
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
