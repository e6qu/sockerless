package main

import "testing"

func TestParseDefaultRouteGatewayIPv4(t *testing.T) {
	route := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t011EA90A\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t001EA90A\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"

	got := parseDefaultRouteGatewayIPv4(route)
	if got != "10.169.30.1" {
		t.Fatalf("default gateway = %q, want 10.169.30.1", got)
	}
}

func TestParseDefaultRouteGatewayIPv4Missing(t *testing.T) {
	got := parseDefaultRouteGatewayIPv4("Iface\tDestination\tGateway\neth0\t001EA90A\t00000000\n")
	if got != "" {
		t.Fatalf("default gateway = %q, want empty", got)
	}
}

func TestRewriteHostDockerInternalEnv(t *testing.T) {
	env := map[string]string{
		"AWS_ENDPOINT_URL": "http://host.docker.internal:4566",
		"UNCHANGED":        "http://example.test",
	}

	got := rewriteHostDockerInternalEnvWithGateway(env, "10.89.30.1")
	if got["UNCHANGED"] != "http://example.test" {
		t.Fatalf("UNCHANGED = %q", got["UNCHANGED"])
	}
	if got["AWS_ENDPOINT_URL"] != "http://10.89.30.1:4566" {
		t.Fatalf("AWS_ENDPOINT_URL = %q", got["AWS_ENDPOINT_URL"])
	}
	if env["AWS_ENDPOINT_URL"] != "http://host.docker.internal:4566" {
		t.Fatalf("input env was mutated: %q", env["AWS_ENDPOINT_URL"])
	}
}
