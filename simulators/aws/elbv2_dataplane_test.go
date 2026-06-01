package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	sim "github.com/sockerless/simulator"
)

func TestELBv2DataPlaneRoutesOnlyLoadBalancerHosts(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := sim.NewServer(sim.Config{Provider: "aws", LogLevel: "disabled"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	registerELBv2(sim.NewAWSQueryRouter(), srv)
	registerELBv2DataPlane(srv)

	var targetHostHeader string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		targetHostHeader = r.Host
		_, _ = w.Write([]byte("elbv2-data-plane"))
	}))
	defer target.Close()
	parsedTargetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse target URL %s: %v", target.URL, err)
	}
	targetHost, targetPortText, err := net.SplitHostPort(parsedTargetURL.Host)
	if err != nil {
		t.Fatalf("target URL host has no port: %s: %v", target.URL, err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("target URL port is not numeric: %s: %v", target.URL, err)
	}

	lb := ELBv2LoadBalancer{
		Arn:     "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/test-lb/abc123",
		Name:    "test-lb",
		DNSName: "test-lb-abc123.elb.us-east-1.amazonaws.com",
		Type:    "application",
	}
	tg := ELBv2TargetGroup{
		Arn:                 "arn:aws:elasticloadbalancing:us-east-1:000000000000:targetgroup/test-tg/abc123",
		Protocol:            "HTTP",
		Port:                80,
		HealthCheckProtocol: "HTTP",
		HealthCheckPath:     "/healthz",
		HealthCheckTimeout:  2,
		Targets: []ELBv2TargetDescription{{
			ID:   targetHost,
			Port: targetPort,
		}},
	}
	listener := ELBv2Listener{
		Arn:             "arn:aws:elasticloadbalancing:us-east-1:000000000000:listener/app/test-lb/abc123/def456",
		LoadBalancerArn: lb.Arn,
		Protocol:        "HTTP",
		Port:            80,
		DefaultActions: []ELBv2Action{{
			Type:           "forward",
			TargetGroupArn: tg.Arn,
		}},
	}
	elbv2LoadBalancers.Put(lb.Arn, lb)
	elbv2TargetGroups.Put(tg.Arn, tg)
	elbv2Listeners.Put(listener.Arn, listener)

	req := httptest.NewRequest(http.MethodGet, "http://simulator/proxy-check", nil)
	req.Host = lb.DNSName
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ELBv2 data-plane status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "elbv2-data-plane" {
		t.Fatalf("ELBv2 data-plane body = %q", rr.Body.String())
	}
	if targetHostHeader != strings.ToLower(lb.DNSName) {
		t.Fatalf("ELBv2 target Host header = %q, want %q", targetHostHeader, strings.ToLower(lb.DNSName))
	}
	if stored, ok := elbv2Listeners.Get(listener.Arn); !ok || stored.Port != listener.Port || stored.LoadBalancerArn != listener.LoadBalancerArn {
		t.Fatalf("ELBv2 data-plane request mutated listener state: %+v found=%t", stored, ok)
	}
	if stored, ok := elbv2LoadBalancers.Get(lb.Arn); !ok || stored.DNSName != lb.DNSName || stored.Arn != lb.Arn {
		t.Fatalf("ELBv2 data-plane request mutated load-balancer state: %+v found=%t", stored, ok)
	}

	unknownHostReq := httptest.NewRequest(http.MethodGet, "http://simulator/proxy-check", nil)
	unknownHostReq.Host = "not-an-elb.localhost"
	unknownHostRR := httptest.NewRecorder()
	srv.ServeHTTP(unknownHostRR, unknownHostReq)
	if unknownHostRR.Code == http.StatusOK && strings.Contains(unknownHostRR.Body.String(), "elbv2-data-plane") {
		t.Fatalf("non-ELB host was routed to ELBv2 data plane")
	}
}

func TestELBv2DataPlaneDoesNotInterceptControlPlaneHost(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := sim.NewServer(sim.Config{Provider: "aws", LogLevel: "disabled"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	registerELBv2(sim.NewAWSQueryRouter(), srv)
	registerELBv2DataPlane(srv)

	srv.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("query-control-plane"))
	})

	req := httptest.NewRequest(http.MethodPost, "http://localhost/", strings.NewReader("Action=DescribeLoadBalancers&Version=2015-12-01"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("control-plane status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "query-control-plane" {
		t.Fatalf("control-plane body = %q", rr.Body.String())
	}
}

func TestELBv2TargetHostHeaderMatchesALBDefaults(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := sim.NewServer(sim.Config{Provider: "aws", LogLevel: "disabled"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	registerELBv2(sim.NewAWSQueryRouter(), srv)

	lb := ELBv2LoadBalancer{
		Arn:        "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/test-lb/abc123",
		Name:       "test-lb",
		DNSName:    "test-lb-abc123.elb.us-east-1.amazonaws.com",
		Type:       "application",
		Attributes: defaultELBv2LoadBalancerAttributes(),
	}
	elbv2LoadBalancers.Put(lb.Arn, lb)

	listener := ELBv2Listener{LoadBalancerArn: lb.Arn, Port: 80}
	if got := elbv2TargetHostHeader("TEST-LB-ABC123.ELB.US-EAST-1.AMAZONAWS.COM", listener); got != strings.ToLower(lb.DNSName) {
		t.Fatalf("default-port Host header = %q", got)
	}

	listener.Port = 8080
	if got := elbv2TargetHostHeader("Example.COM", listener); got != "example.com:8080" {
		t.Fatalf("non-default-port Host header = %q", got)
	}

	lb.Attributes["routing.http.preserve_host_header.enabled"] = "true"
	elbv2LoadBalancers.Put(lb.Arn, lb)
	if got := elbv2TargetHostHeader("Example.COM", listener); got != "Example.COM" {
		t.Fatalf("preserved Host header = %q", got)
	}
}
