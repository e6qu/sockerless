// cloud-map DNS driver shared by every AWS-product backend. Returns
// the Cloud Map private namespace name as the workload's DNS search
// domain. ECS reads its namespace ID per network via NetworkState;
// Lambda will once its DNS path is wired.
//
// Per-backend wiring: backend startup constructs with the
// servicediscovery client + a closure that resolves a network ID to
// the operator-owned namespace ID for that network.

package awscommon

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/sockerless/api"
)

// CloudMapDNS implements core.DNSDriver for the cloud-map mechanism.
type CloudMapDNS struct {
	// Client is the Cloud Map (servicediscovery) SDK client used to
	// resolve namespace ID → namespace name.
	Client *servicediscovery.Client

	// LookupNamespaceID returns the Cloud Map namespace ID for the
	// given network ID. Empty string + nil error means "no namespace
	// configured for this network" (NoOp behaviour for that network).
	LookupNamespaceID func(ctx context.Context, networkID string) (string, error)
}

func (d *CloudMapDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismCloudMap }

func (d *CloudMapDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	if d.LookupNamespaceID == nil || d.Client == nil {
		return "", nil
	}
	id, err := d.LookupNamespaceID(ctx, networkID)
	if err != nil || id == "" {
		return "", err
	}
	out, err := d.Client.GetNamespace(ctx, &servicediscovery.GetNamespaceInput{
		Id: aws.String(id),
	})
	if err != nil || out.Namespace == nil {
		return "", err
	}
	return aws.ToString(out.Namespace.Name), nil
}
