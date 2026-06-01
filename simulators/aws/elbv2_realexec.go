package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func elbv2TargetAddress(tg ELBv2TargetGroup, target ELBv2TargetDescription) (string, error) {
	host := target.ID
	if strings.EqualFold(tg.TargetType, "instance") {
		inst, ok := ec2Instances.Get(target.ID)
		if !ok {
			return "", fmt.Errorf("instance target %s not found", target.ID)
		}
		host = inst.PrivateIpAddress
	}
	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("target %s does not resolve to an IP address", target.ID)
	}
	port := target.Port
	if port == 0 {
		port = tg.Port
	}
	if port == 0 {
		return "", fmt.Errorf("target %s has no port", target.ID)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
