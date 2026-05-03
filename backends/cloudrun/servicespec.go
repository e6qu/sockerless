package cloudrun

import (
	"context"
	"fmt"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
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

	// Multi-container revision: inject SOCKERLESS_HOST_ALIASES into the
	// main container's env so the bootstrap can write `127.0.0.1 <alias>`
	// to /etc/hosts. The aliases are aggregated from every sibling's
	// standard Docker NetworkingConfig.EndpointsConfig.<net>.Aliases —
	// no runner-specific code (the signal is pure Docker API).
	if len(containers) > 1 && len(specs) > 0 {
		members := make([]api.Container, 0, len(containers))
		for _, ci := range containers {
			members = append(members, *ci.Container)
		}
		netID := ""
		if id, ok := s.userDefinedNetworkID(*containers[0].Container); ok {
			netID = id
		}
		aliases := hostAliasesForMembers(members, netID)
		if len(aliases) > 0 {
			specs[0].Env = append(specs[0].Env, &runpb.EnvVar{
				Name:   "SOCKERLESS_HOST_ALIASES",
				Values: &runpb.EnvVar_Value{Value: strings.Join(aliases, ",")},
			})
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
		// ALL_TRAFFIC routes EVERY outbound through the VPC connector.
		// Required so cross-Cloud-Run calls (gitlab-runner-cloudrun POSTing
		// to per-step sockerless-svc-* with Ingress=internal) appear as
		// in-VPC source — Cloud Run rejects same-project Cloud Run
		// requests as "external" if they go via platform egress (public
		// .a.run.app DNS resolves to public IP). With ALL_TRAFFIC + Cloud
		// NAT in the connector subnet, public Google APIs
		// (storage.googleapis.com for GCSFuse, etc.) stay reachable too.
		// Cloud NAT provisioned: sockerless-router / sockerless-nat in
		// sockerless-vpc. See specs/CLOUD_RESOURCE_MAPPING.md § Cloud Run
		// service-to-service call semantics for the full chain.
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
		Labels:      tags.AsGCPLabels(),
		Annotations: tags.AsGCPAnnotations(),
		// BUG-933: Ingress=ALL with IAM-required invoke. Cloud Run rejects
		// cross-project-service-to-service via .a.run.app + Cloud NAT
		// with HTTP 404 because the NAT'd source IP isn't auto-detected
		// as same-project Cloud Run (cell 7 v19 evidence:
		// invokeServiceDefaultCmd POST returned status=404 in 25ms —
		// edge-rejected, never reached the bootstrap).
		//
		// Security stance preserved: NO allUsers→roles/run.invoker
		// binding (verified via service IAM policy). Only the
		// sockerless-runner SA can mint a Cloud Run ID token for this
		// audience — anonymous internet requests get 401 from IAM.
		// Functionally equivalent to ingress=internal+IAM, with the
		// trade-off being a publicly-resolvable URL (DNS leak) that
		// rejects unauthenticated traffic.
		//
		// The right end-state is Cloud Run private DNS via Service
		// Connector + Private Service Connect, which keeps both the
		// URL un-resolvable AND the IAM gate. Deferred — adds Service
		// Connector + per-Service PSC endpoint complexity. Tracked as
		// a follow-up to Phase 122g.
		Ingress:            runpb.IngressTraffic_INGRESS_TRAFFIC_ALL,
		DefaultUriDisabled: false,
		Template:           revTemplate,
	}, nil
}
