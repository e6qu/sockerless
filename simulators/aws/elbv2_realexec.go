package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	realexec "github.com/sockerless/simulator-realexec"
)

var (
	elbv2RealMu      sync.Mutex
	elbv2RealProxies = map[string]*realexec.TCPProxy{}
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

func elbv2ProbeTarget(ctx context.Context, tg ELBv2TargetGroup, target ELBv2TargetDescription) string {
	address, err := elbv2TargetAddress(tg, target)
	if err != nil {
		return "unhealthy"
	}
	protocol := tg.HealthCheckProtocol
	if protocol == "" || strings.EqualFold(protocol, "traffic-port") {
		protocol = tg.Protocol
	}
	timeout := time.Duration(tg.HealthCheckTimeout) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if err := realexec.ProbeTarget(ctx, realexec.ProbeSpec{
		Protocol: protocol,
		Address:  address,
		Path:     tg.HealthCheckPath,
		Timeout:  timeout,
	}); err != nil {
		return "unhealthy"
	}
	return "healthy"
}

func elbv2StartRealListener(listener ELBv2Listener) error {
	elbv2RealMu.Lock()
	if _, ok := elbv2RealProxies[listener.Arn]; ok {
		elbv2RealMu.Unlock()
		return nil
	}
	elbv2RealMu.Unlock()
	proxy, err := realexec.StartTCPProxy(net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Port)), func(ctx context.Context) (string, error) {
		for _, action := range listener.DefaultActions {
			if action.TargetGroupArn == "" {
				continue
			}
			tg, ok := elbv2TargetGroups.Get(action.TargetGroupArn)
			if !ok {
				continue
			}
			for _, target := range tg.Targets {
				if elbv2ProbeTarget(ctx, tg, target) != "healthy" {
					continue
				}
				return elbv2TargetAddress(tg, target)
			}
		}
		return "", fmt.Errorf("no healthy targets")
	})
	if err != nil {
		return err
	}
	elbv2RealMu.Lock()
	elbv2RealProxies[listener.Arn] = proxy
	elbv2RealMu.Unlock()
	return nil
}

func elbv2StopRealListener(listenerArn string) {
	elbv2RealMu.Lock()
	proxy := elbv2RealProxies[listenerArn]
	delete(elbv2RealProxies, listenerArn)
	elbv2RealMu.Unlock()
	if proxy != nil {
		_ = proxy.Close()
	}
}
