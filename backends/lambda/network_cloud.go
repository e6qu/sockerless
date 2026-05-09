package lambda

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
)

// cloudNamespaceCreate provisions a Cloud Map private DNS namespace
// for the given Docker network. Mirror of ecs/service_discovery_cloud.go
// — same `skls-<name>.local` naming, same VPC binding (resolved from
// Config.SubnetIDs's first subnet's VpcId).
//
// Lambda invocations addressed via service-mesh discovery resolve
// peers in this namespace; the per-invocation IP isn't peer-reachable
// (Lambda hyperplane ENIs are shared) so the discovery driver skips
// the register-IP step. Read-only ResolveName works fine.
func (s *Server) cloudNamespaceCreate(ctx context.Context, name, networkID string) error {
	vpcID, err := s.resolveVPCID(ctx)
	if err != nil {
		return fmt.Errorf("resolve VPC ID for namespace: %w", err)
	}

	nsName := "skls-" + name + ".local"

	if existing, err := s.findNamespaceByName(ctx, nsName); err == nil && existing != "" {
		s.Logger.Info().Str("network", name).Str("namespace", existing).Msg("reusing existing Cloud Map namespace")
		s.NetworkState.Update(networkID, func(ns *NetworkState) {
			ns.NamespaceID = existing
		})
		return nil
	}

	nsOut, err := s.aws.ServiceDiscovery.CreatePrivateDnsNamespace(ctx,
		&servicediscovery.CreatePrivateDnsNamespaceInput{
			Name:        aws.String(nsName),
			Vpc:         aws.String(vpcID),
			Description: aws.String(fmt.Sprintf("Sockerless network: %s", name)),
			Tags: []sdtypes.Tag{
				{Key: aws.String("sockerless:network-id"), Value: aws.String(networkID)},
				{Key: aws.String("sockerless:network"), Value: aws.String(name)},
				{Key: aws.String("sockerless-managed"), Value: aws.String("true")},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", nsName, err)
	}

	nsID, err := s.waitForCloudMapOperation(ctx, aws.ToString(nsOut.OperationId))
	if err != nil {
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		ns.NamespaceID = nsID
	})
	s.Logger.Debug().Str("network", name).Str("namespace", nsID).Msg("created Cloud Map namespace")
	return nil
}

// cloudNamespaceDelete tears down the per-network Cloud Map namespace.
func (s *Server) cloudNamespaceDelete(ctx context.Context, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.NamespaceID == "" {
		return nil
	}

	listOut, err := s.aws.ServiceDiscovery.ListServices(ctx,
		&servicediscovery.ListServicesInput{
			Filters: []sdtypes.ServiceFilter{
				{
					Name:      sdtypes.ServiceFilterNameNamespaceId,
					Values:    []string{state.NamespaceID},
					Condition: sdtypes.FilterConditionEq,
				},
			},
		},
	)
	if err == nil {
		for _, svc := range listOut.Services {
			_, _ = s.aws.ServiceDiscovery.DeleteService(ctx,
				&servicediscovery.DeleteServiceInput{Id: svc.Id},
			)
		}
	}

	_, err = s.aws.ServiceDiscovery.DeleteNamespace(ctx,
		&servicediscovery.DeleteNamespaceInput{Id: aws.String(state.NamespaceID)},
	)
	if err != nil {
		return fmt.Errorf("delete namespace %s: %w", state.NamespaceID, err)
	}
	s.NetworkState.Delete(networkID)
	s.Logger.Debug().Str("namespace", state.NamespaceID).Msg("deleted Cloud Map namespace")
	return nil
}

// resolveVPCID returns the VPC ID hosting the configured Lambda subnets.
func (s *Server) resolveVPCID(ctx context.Context) (string, error) {
	if len(s.config.SubnetIDs) == 0 {
		return "", fmt.Errorf("no subnets configured (SOCKERLESS_LAMBDA_SUBNETS) — service-mesh discovery requires VPC mode")
	}
	out, err := s.aws.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: s.config.SubnetIDs[:1],
	})
	if err != nil {
		return "", err
	}
	if len(out.Subnets) == 0 {
		return "", fmt.Errorf("subnet %s not found", s.config.SubnetIDs[0])
	}
	return aws.ToString(out.Subnets[0].VpcId), nil
}

// findNamespaceByName looks up a Cloud Map namespace by exact name and
// returns its ID, or "" if not found. Used for idempotent creation.
func (s *Server) findNamespaceByName(ctx context.Context, name string) (string, error) {
	listOut, err := s.aws.ServiceDiscovery.ListNamespaces(ctx,
		&servicediscovery.ListNamespacesInput{
			Filters: []sdtypes.NamespaceFilter{
				{
					Name:      sdtypes.NamespaceFilterNameType,
					Values:    []string{string(sdtypes.NamespaceTypeDnsPrivate)},
					Condition: sdtypes.FilterConditionEq,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}
	for _, ns := range listOut.Namespaces {
		if aws.ToString(ns.Name) == name {
			return aws.ToString(ns.Id), nil
		}
	}
	return "", nil
}

// waitForCloudMapOperation polls GetOperation until SUCCESS or FAIL,
// returning the resource ID on success. Mirrors ecs.waitForOperation
// (intentional duplication — Lambda's namespace lifecycle is simpler
// so the helper stays local).
func (s *Server) waitForCloudMapOperation(ctx context.Context, operationID string) (string, error) {
	for i := 0; i < 60; i++ {
		out, err := s.aws.ServiceDiscovery.GetOperation(ctx,
			&servicediscovery.GetOperationInput{OperationId: aws.String(operationID)},
		)
		if err != nil {
			return "", err
		}
		switch out.Operation.Status {
		case sdtypes.OperationStatusSuccess:
			if id, ok := out.Operation.Targets["NAMESPACE"]; ok {
				return id, nil
			}
			return "", fmt.Errorf("operation %s succeeded without NAMESPACE target", operationID)
		case sdtypes.OperationStatusFail:
			return "", fmt.Errorf("operation %s failed: %s", operationID, aws.ToString(out.Operation.ErrorMessage))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(s.config.PollInterval):
		}
	}
	return "", fmt.Errorf("timeout waiting for operation %s", operationID)
}
