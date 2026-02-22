package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func computeService(t *testing.T) *compute.Service {
	t.Helper()
	svc, err := compute.NewService(ctx,
		option.WithEndpoint(baseURL+"/compute/v1/"),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func TestCompute_CreateNetwork(t *testing.T) {
	svc := computeService(t)

	network := &compute.Network{
		Name:                  "test-network",
		AutoCreateSubnetworks: false,
	}
	_, err := svc.Networks.Insert("test-project", network).Do()
	require.NoError(t, err)

	got, err := svc.Networks.Get("test-project", "test-network").Do()
	require.NoError(t, err)
	assert.Equal(t, "test-network", got.Name)
}

func TestCompute_CreateSubnetwork(t *testing.T) {
	svc := computeService(t)

	// Create network first
	network := &compute.Network{
		Name:                  "subnet-test-net",
		AutoCreateSubnetworks: false,
	}
	_, err := svc.Networks.Insert("test-project", network).Do()
	require.NoError(t, err)

	subnet := &compute.Subnetwork{
		Name:        "test-subnet",
		IpCidrRange: "10.0.0.0/24",
		Network:     "projects/test-project/global/networks/subnet-test-net",
		Region:      "us-central1",
	}
	_, err = svc.Subnetworks.Insert("test-project", "us-central1", subnet).Do()
	require.NoError(t, err)

	got, err := svc.Subnetworks.Get("test-project", "us-central1", "test-subnet").Do()
	require.NoError(t, err)
	assert.Equal(t, "test-subnet", got.Name)
}

func TestCompute_ListNetworks(t *testing.T) {
	svc := computeService(t)

	network := &compute.Network{
		Name:                  "list-net",
		AutoCreateSubnetworks: false,
	}
	_, err := svc.Networks.Insert("test-project", network).Do()
	require.NoError(t, err)

	resp, err := svc.Networks.List("test-project").Do()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Items), 1)
}
