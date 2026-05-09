// Cloud-Map DNS driver for the ECS backend. Returns the Cloud Map
// private namespace name as the workload's DNS search domain.

package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/sockerless/api"
)

type cloudMapDNS struct {
	s *Server
}

func newCloudMapDNS(s *Server) *cloudMapDNS {
	return &cloudMapDNS{s: s}
}

func (d *cloudMapDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	state, ok := d.s.NetworkState.Get(networkID)
	if !ok || state.NamespaceID == "" {
		return "", nil
	}
	out, err := d.s.aws.ServiceDiscovery.GetNamespace(ctx, &servicediscovery.GetNamespaceInput{
		Id: aws.String(state.NamespaceID),
	})
	if err != nil || out.Namespace == nil {
		return "", err
	}
	return aws.ToString(out.Namespace.Name), nil
}

func (d *cloudMapDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismCloudMap }
