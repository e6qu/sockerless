package cloudrun

import (
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
func (s *Server) buildServiceSpec(containers []containerInput) *runpb.Service {
	var specs []*runpb.Container
	for _, ci := range containers {
		specs = append(specs, s.buildContainerSpec(ci))
	}

	revTemplate := &runpb.RevisionTemplate{
		Containers: specs,
		Scaling: &runpb.RevisionScaling{
			MinInstanceCount: 1,
			MaxInstanceCount: 1,
		},
		Timeout: durationpb.New(1 * time.Hour),
	}

	if s.config.VPCConnector != "" {
		revTemplate.VpcAccess = &runpb.VpcAccess{
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
	}
	// Carry pod membership through to Service labels so ListPods can
	// reconstruct docker pods after a restart.
	if pod, _ := s.Store.Pods.GetPodForContainer(containers[0].ID); pod != nil {
		tags.Pod = pod.Name
	}

	return &runpb.Service{
		Labels:             tags.AsGCPLabels(),
		Annotations:        tags.AsGCPAnnotations(),
		Ingress:            runpb.IngressTraffic_INGRESS_TRAFFIC_INTERNAL_ONLY,
		DefaultUriDisabled: true,
		Template:           revTemplate,
	}
}
