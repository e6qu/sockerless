package gcp_cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lean gcloud-compute coverage for the control-plane CRUD added in
// compute_more.go: a global external address, a regional target pool, and
// the regions catalog read — each round-trips through the real gcloud CLI.

func TestGcloudComputeGlobalAddress_CRUD(t *testing.T) {
	name := "sim-gaddr-cli-1"
	out, err := gcloudCLI("compute", "addresses", "create", name,
		"--global", "--format=value(name)").CombinedOutput()
	require.NoError(t, err, "create: %s", out)

	out, err = gcloudCLI("compute", "addresses", "describe", name,
		"--global", "--format=value(name)").CombinedOutput()
	require.NoError(t, err, "describe: %s", out)
	assert.Contains(t, string(out), name)

	out, err = gcloudCLI("compute", "addresses", "list", "--global",
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "list: %s", out)
	assert.Contains(t, string(out), name)

	out, err = gcloudCLI("compute", "addresses", "delete", name,
		"--global", "--quiet").CombinedOutput()
	require.NoError(t, err, "delete: %s", out)
}

func TestGcloudComputeTargetPool_CRUD(t *testing.T) {
	region := "us-central1"
	name := "sim-tp-cli-1"
	out, err := gcloudCLI("compute", "target-pools", "create", name,
		"--region="+region, "--format=value(name)").CombinedOutput()
	require.NoError(t, err, "create: %s", out)

	out, err = gcloudCLI("compute", "target-pools", "describe", name,
		"--region="+region, "--format=value(name)").CombinedOutput()
	require.NoError(t, err, "describe: %s", out)
	assert.Contains(t, string(out), name)

	out, err = gcloudCLI("compute", "target-pools", "delete", name,
		"--region="+region, "--quiet").CombinedOutput()
	require.NoError(t, err, "delete: %s", out)
}

func TestGcloudComputeRegions_List(t *testing.T) {
	out, err := gcloudCLI("compute", "regions", "list",
		"--format=value(name)").CombinedOutput()
	require.NoError(t, err, "list: %s", out)
	assert.True(t, strings.Contains(string(out), "us-central1"), "regions: %s", out)
}
