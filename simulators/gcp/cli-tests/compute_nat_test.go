package gcp_cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGcloudComputeAddressAndRouterNAT(t *testing.T) {
	requireNetworkHost(t)
	region := "us-central1"
	network := "cli-nat-network"
	address := "cli-nat-address"
	router := "cli-nat-router"
	nat := "cli-manual-nat"

	out, err := gcloudCLI("compute", "networks", "create", network,
		"--subnet-mode=custom",
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "network create: %s", out)

	out, err = gcloudCLI("compute", "addresses", "create", address,
		"--region="+region,
		"--network-tier=PREMIUM",
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "address create: %s", out)

	out, err = gcloudCLI("compute", "addresses", "describe", address,
		"--region="+region,
		"--format=value(name,status,address)").CombinedOutput()
	require.NoError(t, err, "address describe: %s", out)
	body := strings.ToLower(string(out))
	require.Contains(t, body, address)
	require.Contains(t, body, "reserved")

	out, err = gcloudCLI("compute", "routers", "create", router,
		"--region="+region,
		"--network="+network,
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "router create: %s", out)

	out, err = gcloudCLI("compute", "routers", "nats", "create", nat,
		"--router="+router,
		"--region="+region,
		"--nat-external-ip-pool="+address,
		"--nat-custom-subnet-ip-ranges=all",
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "nat create: %s", out)

	out, err = gcloudCLI("compute", "routers", "get-status", router,
		"--region="+region,
		"--format=json").CombinedOutput()
	require.NoError(t, err, "router get-status: %s", out)

	out, err = gcloudCLI("compute", "routers", "nats", "delete", nat,
		"--router="+router,
		"--region="+region,
		"--quiet").CombinedOutput()
	require.NoError(t, err, "nat delete: %s", out)

	out, err = gcloudCLI("compute", "routers", "delete", router,
		"--region="+region,
		"--quiet").CombinedOutput()
	require.NoError(t, err, "router delete: %s", out)

	out, err = gcloudCLI("compute", "addresses", "delete", address,
		"--region="+region,
		"--quiet").CombinedOutput()
	require.NoError(t, err, "address delete: %s", out)
}
