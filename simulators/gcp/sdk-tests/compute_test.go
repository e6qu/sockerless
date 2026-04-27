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

// TestCompute_Firewall_CreateGetListDelete pins the firewall rule
// surface that runner setup flows hit. Real GCP rejects unknown
// directions / negative priorities; the sim defaults to INGRESS +
// priority=1000 like real GCP.
func TestCompute_Firewall_CreateGetListDelete(t *testing.T) {
	svc := computeService(t)

	rule := &compute.Firewall{
		Name:         "fw-allow-runner-ingress",
		Network:      "projects/test-project/global/networks/test-network",
		Direction:    "INGRESS",
		Priority:     900,
		SourceRanges: []string{"10.0.0.0/8"},
		Allowed: []*compute.FirewallAllowed{
			{IPProtocol: "tcp", Ports: []string{"22", "80", "443"}},
		},
	}

	op, err := svc.Firewalls.Insert("test-project", rule).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "DONE", op.Status)

	got, err := svc.Firewalls.Get("test-project", rule.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, rule.Name, got.Name)
	assert.Equal(t, "INGRESS", got.Direction)
	assert.Equal(t, int64(900), got.Priority)
	require.Len(t, got.Allowed, 1)
	assert.Equal(t, "tcp", got.Allowed[0].IPProtocol)
	assert.ElementsMatch(t, []string{"22", "80", "443"}, got.Allowed[0].Ports)

	listOut, err := svc.Firewalls.List("test-project").Context(ctx).Do()
	require.NoError(t, err)
	found := false
	for _, fw := range listOut.Items {
		if fw.Name == rule.Name {
			found = true
			break
		}
	}
	assert.True(t, found, "firewall must show up in List")

	_, err = svc.Firewalls.Delete("test-project", rule.Name).Context(ctx).Do()
	require.NoError(t, err)

	_, err = svc.Firewalls.Get("test-project", rule.Name).Context(ctx).Do()
	require.Error(t, err, "Get after Delete must 404")
}

func TestCompute_Firewall_DefaultsToIngressPriority1000(t *testing.T) {
	svc := computeService(t)

	rule := &compute.Firewall{
		Name:    "fw-defaults",
		Network: "projects/test-project/global/networks/test-network",
		Allowed: []*compute.FirewallAllowed{
			{IPProtocol: "icmp"},
		},
	}
	_, err := svc.Firewalls.Insert("test-project", rule).Context(ctx).Do()
	require.NoError(t, err)
	defer svc.Firewalls.Delete("test-project", rule.Name).Context(ctx).Do()

	got, err := svc.Firewalls.Get("test-project", rule.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "INGRESS", got.Direction, "default direction must match real GCP")
	assert.Equal(t, int64(1000), got.Priority, "default priority must match real GCP")
}
