package ecs

import (
	"fmt"
	"net"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// extractENIIP extracts the private IP address from a Fargate task's ENI attachment.
func extractENIIP(task ecstypes.Task) string {
	for _, attachment := range task.Attachments {
		if attachment.Type == nil || *attachment.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, detail := range attachment.Details {
			if detail.Name == nil || detail.Value == nil {
				continue
			}
			if *detail.Name == "privateIPv4Address" {
				return *detail.Value
			}
		}
	}
	return ""
}

// deriveMACFromIP generates a Docker-convention MAC address from an IP.
// Format: 02:42:XX:XX:XX:XX where XX are the 4 IP octets in hex.
// Derives MAC from real IP instead of using a hardcoded value.
func deriveMACFromIP(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "02:42:ac:11:00:02"
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "02:42:ac:11:00:02"
	}
	return fmt.Sprintf("02:42:%02x:%02x:%02x:%02x", ip4[0], ip4[1], ip4[2], ip4[3])
}
