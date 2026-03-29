package ecs

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// cloudNetworkCreate creates a VPC security group for a Docker network.
// The security group has a self-referencing ingress rule so containers
// in the same network can communicate freely.
func (s *Server) cloudNetworkCreate(name, networkID string) error {
	groupName := "skls-" + name

	// Look up VPC ID from the first configured subnet.
	vpcID, err := s.resolveVPCID()
	if err != nil {
		s.Logger.Error().Err(err).Msg("failed to resolve VPC ID for network security group")
		return fmt.Errorf("failed to resolve VPC ID: %w", err)
	}
	s.Logger.Info().Str("vpc", vpcID).Str("network", name).Msg("creating security group for Docker network")

	// Create the security group.
	createOut, err := s.aws.EC2.CreateSecurityGroup(s.ctx(), &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(groupName),
		Description: aws.String(fmt.Sprintf("Sockerless network: %s", name)),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeSecurityGroup,
				Tags: []ec2types.Tag{
					{Key: aws.String("sockerless:network"), Value: aws.String(name)},
					{Key: aws.String("sockerless:network-id"), Value: aws.String(networkID)},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create security group %s: %w", groupName, err)
	}

	sgID := aws.ToString(createOut.GroupId)
	s.Logger.Info().
		Str("network", name).
		Str("sg", sgID).
		Str("vpc", vpcID).
		Msg("created security group for network")

	// Add self-referencing ingress rule: allow all traffic from the same SG.
	_, err = s.aws.EC2.AuthorizeSecurityGroupIngress(s.ctx(), &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(sgID),
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("-1"), // all protocols
				UserIdGroupPairs: []ec2types.UserIdGroupPair{
					{GroupId: aws.String(sgID)},
				},
			},
		},
	})
	if err != nil {
		s.Logger.Warn().Err(err).Str("sg", sgID).Msg("failed to add self-referencing ingress rule")
	}

	// Store the security group ID in network state.
	s.NetworkState.Put(networkID, NetworkState{
		SecurityGroupID: sgID,
	})

	return nil
}

// cloudNetworkDelete deletes the VPC security group for a Docker network.
func (s *Server) cloudNetworkDelete(networkID string) error {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.SecurityGroupID == "" {
		return nil // no cloud resources to clean up
	}

	_, err := s.aws.EC2.DeleteSecurityGroup(s.ctx(), &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(ns.SecurityGroupID),
	})
	if err != nil {
		return fmt.Errorf("failed to delete security group %s: %w", ns.SecurityGroupID, err)
	}

	s.Logger.Debug().
		Str("sg", ns.SecurityGroupID).
		Msg("deleted security group for network")

	s.NetworkState.Delete(networkID)
	return nil
}

// cloudNetworkConnect associates a network's security group with a container.
// Appends to SecurityGroupIDs (supports multiple networks).
func (s *Server) cloudNetworkConnect(networkID, containerID string) error {
	ns, ok := s.NetworkState.Get(networkID)
	if !ok || ns.SecurityGroupID == "" {
		return fmt.Errorf("network %s has no associated security group", networkID)
	}

	s.ECS.Update(containerID, func(state *ECSState) {
		// Dedup: don't add the same SG twice
		for _, sg := range state.SecurityGroupIDs {
			if sg == ns.SecurityGroupID {
				return
			}
		}
		state.SecurityGroupIDs = append(state.SecurityGroupIDs, ns.SecurityGroupID)
	})

	s.Logger.Debug().
		Str("container", containerID[:12]).
		Str("sg", ns.SecurityGroupID).
		Msg("associated security group with container")

	return nil
}

// cloudNetworkDisconnect removes a network security group association
// from a container's ECS state.
// Removes specific SG from SecurityGroupIDs slice.
func (s *Server) cloudNetworkDisconnect(networkID, containerID string) error {
	ns, _ := s.NetworkState.Get(networkID)

	s.ECS.Update(containerID, func(state *ECSState) {
		if ns.SecurityGroupID == "" {
			return
		}
		filtered := state.SecurityGroupIDs[:0]
		for _, sg := range state.SecurityGroupIDs {
			if sg != ns.SecurityGroupID {
				filtered = append(filtered, sg)
			}
		}
		state.SecurityGroupIDs = filtered
	})

	s.Logger.Debug().
		Str("container", containerID[:12]).
		Str("network", networkID).
		Msg("removed security group association from container")

	return nil
}

// resolveVPCID determines the VPC ID from the first configured subnet.
func (s *Server) resolveVPCID() (string, error) {
	if len(s.config.Subnets) == 0 {
		return "", fmt.Errorf("no subnets configured")
	}

	result, err := s.aws.EC2.DescribeSubnets(s.ctx(), &ec2.DescribeSubnetsInput{
		SubnetIds: s.config.Subnets[:1],
	})
	if err != nil {
		return "", err
	}
	if len(result.Subnets) == 0 {
		return "", fmt.Errorf("subnet %s not found", s.config.Subnets[0])
	}

	return aws.ToString(result.Subnets[0].VpcId), nil
}
