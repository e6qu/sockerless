package cloudrun

import (
	"context"
	"fmt"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
	"google.golang.org/protobuf/types/known/durationpb"
)

// buildServiceName generates a Cloud Run Service name from a container ID.
// Distinct prefix from buildJobName so Jobs and Services never collide in
// the same project when UseService is toggled across containers.
func buildServiceName(containerID string) string {
	return fmt.Sprintf("sockerless-svc-%s", containerID[:12])
}

// buildServiceParent returns the Cloud Run parent resource path for Services.
// Identical to the Jobs parent; Service resources are siblings of Jobs.
func (s *Server) buildServiceParent() string {
	return fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
}

// buildServiceSpec creates a Cloud Run Service protobuf from one or more
// containers.: Services replace Jobs for the cross-container DNS
// path because Services have addressable per-revision internal IPs (via
// VPC connector + internal ingress) whereas Jobs do not.
// Callers are expected to have verified s.config.VPCConnector is non-empty
// and s.config.UseService is true; this builder does not enforce that.
func (s *Server) buildServiceSpec(ctx context.Context, containers []containerInput) (*runpb.Service, error) {
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

	revTemplate := &runpb.RevisionTemplate{
		Containers: specs,
		Volumes:    volumes,
		Scaling: &runpb.RevisionScaling{
			MinInstanceCount: 1,
			MaxInstanceCount: 1,
		},
		Timeout: durationpb.New(1 * time.Hour),
	}

	if s.config.VPCConnector != "" {
		// PRIVATE_RANGES_ONLY: only RFC1918 traffic (cross-container
		// peer-to-peer via VPC) goes through the connector. Public
		// google APIs (storage.googleapis.com — required by GCSFuse for
		// GCS volume mounts; cloudrun control plane invokes; etc.) go
		// via the platform's normal egress so they don't depend on
		// in-VPC NAT or Private Google Access. Phase 122g BUG-928 —
		// ALL_TRAFFIC routed GCSFuse through the connector subnet which
		// has no NAT, breaking every Service-path container start.
		revTemplate.VpcAccess = &runpb.VpcAccess{
			Connector: s.config.VPCConnector,
			Egress:    runpb.VpcAccess_PRIVATE_RANGES_ONLY,
		}
	}

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "cloudrun",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		Name:        containers[0].Container.Name,
		Network:     containers[0].Container.HostConfig.NetworkMode,
	}
	// Carry pod membership through to Service labels so ListPods can
	// reconstruct docker pods after a restart.
	if pod, _ := s.Store.Pods.GetPodForContainer(containers[0].ID); pod != nil {
		tags.Pod = pod.Name
	}

	return &runpb.Service{
		Labels:      tags.AsGCPLabels(),
		Annotations: tags.AsGCPAnnotations(),
		Ingress:     runpb.IngressTraffic_INGRESS_TRAFFIC_INTERNAL_ONLY,
		// Phase 122g BUG-931: keep the default URL enabled so the
		// backend can POST envelope payloads to the bootstrap (Path B).
		// Ingress=internal still restricts who can hit it — only
		// callers from the same VPC connector. Without this, the
		// Service has no addressable URL and ContainerWait blocks
		// forever (BUG-929 v2 evidence: cell 7 v15 logs show
		// 'sockerless-cloudrun-bootstrap: listening on :8080' but the
		// invoke goroutine sees status.url empty for the entire
		// 5-minute waitForServiceURL window).
		DefaultUriDisabled: false,
		Template:           revTemplate,
	}, nil
}
