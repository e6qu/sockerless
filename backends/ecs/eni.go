package ecs

import (
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// extractENIIP extracts the private IP address from a Fargate task's ENI attachment.
func extractENIIP(task ecstypes.Task) string {
	for _, attachment := range task.Attachments {
		if *attachment.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, detail := range attachment.Details {
			if *detail.Name == "privateIPv4Address" {
				return *detail.Value
			}
		}
	}
	return ""
}
