package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	realexec "github.com/sockerless/simulator-realexec"
)

// elbv2NLBProxies tracks one real TCP proxy per Network Load Balancer listener
// that speaks a stream protocol (TCP / TCP_UDP). A real NLB forwards the raw
// byte stream from the listener to a healthy registered target without parsing
// HTTP; the sim realizes that by binding a real listener socket and io.Copy-ing
// both directions to the target. The ALB/HTTP data plane (elbv2_dataplane.go)
// stays HTTP-only; NLB stream listeners go through this path instead.
var (
	elbv2NLBProxyMu sync.Mutex
	elbv2NLBProxies = map[string]*realexec.TCPProxy{}
)

// elbv2ListenerIsStream reports whether a listener forwards a raw byte stream
// (NLB TCP / TCP_UDP) rather than HTTP. TLS and UDP are excluded: TLS terminates
// at the listener (needs a cert + handshake the sim doesn't model) and UDP is
// not a stream, so neither is faithfully a raw-TCP byte proxy.
func elbv2ListenerIsStream(listener ELBv2Listener) bool {
	switch strings.ToUpper(listener.Protocol) {
	case "TCP", "TCP_UDP":
		return true
	default:
		return false
	}
}

// elbv2StartNLBProxy binds a real TCP listener for an NLB stream listener and
// forwards every accepted connection to a healthy registered target, chosen at
// connect time (so target (de)registration and health are honored per
// connection, like a real NLB). It binds 0.0.0.0:<ephemeral> so the proxy is
// reachable both from loopback (in-process tests) and from workload containers
// via the host gateway. Idempotent per listener ARN.
func elbv2StartNLBProxy(listener ELBv2Listener) error {
	if !elbv2ListenerIsStream(listener) {
		return nil
	}
	elbv2NLBProxyMu.Lock()
	defer elbv2NLBProxyMu.Unlock()
	if _, ok := elbv2NLBProxies[listener.Arn]; ok {
		return nil
	}
	listenerArn := listener.Arn
	resolver := func(ctx context.Context) (string, error) {
		current, ok := elbv2Listeners.Get(listenerArn)
		if !ok {
			return "", fmt.Errorf("listener %s no longer exists", listenerArn)
		}
		tg, target, ok := elbv2HealthyTargetForListener(ctx, current)
		if !ok {
			return "", fmt.Errorf("no healthy targets for listener %s", listenerArn)
		}
		return elbv2TargetAddress(tg, target)
	}
	proxy, err := realexec.StartTCPProxy("0.0.0.0:0", resolver)
	if err != nil {
		return fmt.Errorf("start NLB TCP proxy for listener %s: %w", listenerArn, err)
	}
	elbv2NLBProxies[listenerArn] = proxy
	return nil
}

// elbv2StopNLBProxy closes and forgets the TCP proxy for a listener (on
// DeleteListener / DeleteLoadBalancer). No-op if none is running.
func elbv2StopNLBProxy(listenerArn string) {
	elbv2NLBProxyMu.Lock()
	proxy := elbv2NLBProxies[listenerArn]
	delete(elbv2NLBProxies, listenerArn)
	elbv2NLBProxyMu.Unlock()
	if proxy != nil {
		_ = proxy.Close()
	}
}

// elbv2NLBProxyAddress returns the host:port a client connects to in order to
// reach an NLB stream listener's data plane. A real NLB exposes the listener on
// the load balancer's DNS name at the listener port; the sim binds an ephemeral
// port instead, so this address is the coordinate a client uses to reach the
// listener. The bind host (0.0.0.0) is rewritten to a dialable host reachable
// both from in-process clients and from workload containers via the host
// gateway. Empty string if no proxy is running for the listener.
func elbv2NLBProxyAddress(listenerArn string) string {
	elbv2NLBProxyMu.Lock()
	defer elbv2NLBProxyMu.Unlock()
	proxy := elbv2NLBProxies[listenerArn]
	if proxy == nil {
		return ""
	}
	return elbv2DialableProxyAddress(proxy.Address)
}

// elbv2DialableProxyAddress rewrites a 0.0.0.0:<port> / [::]:<port> bind address
// (what net.Listener reports for an any-interface bind) into a concrete dialable
// host:port a real client can connect to. The proxy binds every interface, so a
// same-host client (an SDK/CLI client talking to the sim host process) reaches
// it on loopback, while a workload container reaches it on the sim host's
// routable IP (the same host coordinate workloadCallbackHost resolves for the
// IMDS/runtime endpoints).
func elbv2DialableProxyAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if runningInsideContainer() {
			if ip := firstNonLoopbackIPv4(); ip != "" {
				host = ip
			} else {
				host = "127.0.0.1"
			}
		} else {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, port)
}

// elbv2NLBStreamEndpoint returns the dialable host:port for a load balancer's
// stream (NLB TCP / TCP_UDP) data plane, or empty if the load balancer has no
// running stream-listener proxy. This is the production discovery coordinate a
// client uses to reach an NLB the way it dials a real NLB's DNS endpoint; it is
// surfaced through DescribeLoadBalancers (as the reachable DNSName) so a real
// client never needs the private proxy map.
func elbv2NLBStreamEndpoint(lbArn string) string {
	for _, listener := range elbv2Listeners.Filter(func(l ELBv2Listener) bool {
		return l.LoadBalancerArn == lbArn && elbv2ListenerIsStream(l)
	}) {
		if addr := elbv2NLBProxyAddress(listener.Arn); addr != "" {
			return addr
		}
	}
	return ""
}

// elbv2ReportedDNSName returns the DNSName DescribeLoadBalancers reports for a
// load balancer. A real NLB's reachable endpoint is its DNS name; the sim binds
// the stream data plane on an ephemeral port, so for a Network Load Balancer
// that has a running stream listener the reported DNSName is the dialable
// host:port a client connects to (`net.Dial(dnsName)`), exactly as it would dial
// a real NLB's DNS endpoint. Application Load Balancers keep their AWS-shaped
// DNS name (their HTTP data plane is reached by Host header), and an NLB with no
// stream listener yet keeps its AWS-shaped name too.
func elbv2ReportedDNSName(lb ELBv2LoadBalancer) string {
	if lb.Type == "network" {
		if endpoint := elbv2NLBStreamEndpoint(lb.Arn); endpoint != "" {
			return endpoint
		}
	}
	return lb.DNSName
}
