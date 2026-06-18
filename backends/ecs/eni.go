package ecs

import (
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// eniDetail returns the value of a named ENI-attachment detail on a Fargate
// task, or "" if the attachment hasn't reported it.
func eniDetail(task ecstypes.Task, name string) string {
	for _, attachment := range task.Attachments {
		if attachment.Type == nil || *attachment.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, detail := range attachment.Details {
			if detail.Name == nil || detail.Value == nil {
				continue
			}
			if *detail.Name == name {
				return *detail.Value
			}
		}
	}
	return ""
}

// extractENIIP extracts the private IP address from a Fargate task's ENI attachment.
func extractENIIP(task ecstypes.Task) string {
	return eniDetail(task, "privateIPv4Address")
}

// extractENIMAC extracts the real MAC address the Fargate task's ENI attachment
// reports. Returns "" when the attachment hasn't reported one yet — the MAC is
// never synthesized from the IP.
func extractENIMAC(task ecstypes.Task) string {
	return eniDetail(task, "macAddress")
}
