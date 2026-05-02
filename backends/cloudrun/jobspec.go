package cloudrun

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
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

	if config.WorkingDir != "" {
		containerSpec.WorkingDir = config.WorkingDir
	}

	return containerSpec, mounts
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
					Gcs: &runpb.GCSVolumeSource{Bucket: bucket},
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

// sanitizeContainerName converts a container name to a valid Cloud Run container name.
// Strips leading "/" and replaces non-alphanumeric characters with "-".
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
