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

func TestCompute_RegionalAddressAndManualRouterNAT(t *testing.T) {
	svc := computeService(t)
	const project = "test-project"
	const region = "us-central1"

	network := &compute.Network{
		Name:                  "sdk-nat-network",
		AutoCreateSubnetworks: false,
	}
	_, err := svc.Networks.Insert(project, network).Context(ctx).Do()
	require.NoError(t, err)
	subnet := &compute.Subnetwork{
		Name:        "sdk-nat-subnet",
		IpCidrRange: "10.25.0.0/24",
		Network:     "projects/test-project/global/networks/sdk-nat-network",
		Region:      region,
	}
	_, err = svc.Subnetworks.Insert(project, region, subnet).Context(ctx).Do()
	require.NoError(t, err)

	addr := &compute.Address{
		Name:        "sdk-nat-address",
		AddressType: "EXTERNAL",
		IpVersion:   "IPV4",
		NetworkTier: "PREMIUM",
	}
	_, err = svc.Addresses.Insert(project, region, addr).Context(ctx).Do()
	require.NoError(t, err)

	gotAddr, err := svc.Addresses.Get(project, region, addr.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, addr.Name, gotAddr.Name)
	assert.Equal(t, "RESERVED", gotAddr.Status)
	assert.NotEmpty(t, gotAddr.Address)

	addrList, err := svc.Addresses.List(project, region).Context(ctx).Do()
	require.NoError(t, err)
	require.NotEmpty(t, addrList.Items)

	router := &compute.Router{
		Name:    "sdk-nat-router",
		Network: "projects/test-project/global/networks/sdk-nat-network",
		Nats: []*compute.RouterNat{{
			Name:                          "sdk-manual-nat",
			NatIpAllocateOption:           "MANUAL_ONLY",
			NatIps:                        []string{gotAddr.SelfLink},
			SourceSubnetworkIpRangesToNat: "ALL_SUBNETWORKS_ALL_IP_RANGES",
		}},
	}
	_, err = svc.Routers.Insert(project, region, router).Context(ctx).Do()
	require.NoError(t, err)

	gotRouter, err := svc.Routers.Get(project, region, router.Name).Context(ctx).Do()
	require.NoError(t, err)
	require.Len(t, gotRouter.Nats, 1)
	assert.Equal(t, "MANUAL_ONLY", gotRouter.Nats[0].NatIpAllocateOption)
	assert.Equal(t, gotAddr.SelfLink, gotRouter.Nats[0].NatIps[0])

	status, err := svc.Routers.GetRouterStatus(project, region, router.Name).Context(ctx).Do()
	require.NoError(t, err)
	require.NotNil(t, status.Result)

	_, err = svc.Routers.Delete(project, region, router.Name).Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.Addresses.Delete(project, region, addr.Name).Context(ctx).Do()
	require.NoError(t, err)
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

// TestCompute_Disks_CRUD covers the GCP `pd-ephemeral` storage-driver
// prereq: zonal Compute Disks insert / get / list /
// resize / setLabels / delete + aggregated list across zones. Real
// GCP returns zonal operations for every mutation; the sim's ops
// endpoint always reports DONE so the SDK's polling loop completes
// in one round.
func TestCompute_Disks_CRUD(t *testing.T) {
	svc := computeService(t)
	const project = "test-project"
	const zone = "us-central1-a"

	d := &compute.Disk{
		Name:        "ephemeral-1",
		SizeGb:      20,
		Description: "phase-127 pd-ephemeral test disk",
	}
	_, err := svc.Disks.Insert(project, zone, d).Context(ctx).Do()
	require.NoError(t, err)

	got, err := svc.Disks.Get(project, zone, "ephemeral-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "ephemeral-1", got.Name)
	assert.Equal(t, int64(20), got.SizeGb)
	assert.Equal(t, "READY", got.Status)
	assert.Contains(t, got.Type, "diskTypes/pd-standard", "default type when unset")

	list, err := svc.Disks.List(project, zone).Context(ctx).Do()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list.Items), 1)

	_, err = svc.Disks.Resize(project, zone, "ephemeral-1",
		&compute.DisksResizeRequest{SizeGb: 50}).Context(ctx).Do()
	require.NoError(t, err)
	resized, err := svc.Disks.Get(project, zone, "ephemeral-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, int64(50), resized.SizeGb, "resize must update size")

	_, err = svc.Disks.SetLabels(project, zone, "ephemeral-1",
		&compute.ZoneSetLabelsRequest{
			Labels:           map[string]string{"sockerless_session": "abc123"},
			LabelFingerprint: resized.LabelFingerprint,
		}).Context(ctx).Do()
	require.NoError(t, err)
	labelled, err := svc.Disks.Get(project, zone, "ephemeral-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "abc123", labelled.Labels["sockerless_session"], "setLabels round-trip")

	_, err = svc.Disks.Delete(project, zone, "ephemeral-1").Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.Disks.Get(project, zone, "ephemeral-1").Context(ctx).Do()
	require.Error(t, err, "get after delete must fail")
}

// TestCompute_Disks_AggregatedList covers the cross-zone aggregated
// surface terraform's `data "google_compute_disks"` uses. Real GCP
// groups by `zones/<zone>` keys; sim mirrors that shape.
func TestCompute_Disks_AggregatedList(t *testing.T) {
	svc := computeService(t)
	const project = "test-project"

	for _, z := range []string{"us-central1-a", "us-east1-b"} {
		_, err := svc.Disks.Insert(project, z, &compute.Disk{
			Name:   "agg-" + z,
			SizeGb: 10,
		}).Context(ctx).Do()
		require.NoError(t, err)
	}

	agg, err := svc.Disks.AggregatedList(project).Context(ctx).Do()
	require.NoError(t, err)
	require.NotEmpty(t, agg.Items)

	foundCentral := false
	foundEast := false
	for key, scoped := range agg.Items {
		for _, d := range scoped.Disks {
			if d.Name == "agg-us-central1-a" {
				assert.Equal(t, "zones/us-central1-a", key)
				foundCentral = true
			}
			if d.Name == "agg-us-east1-b" {
				assert.Equal(t, "zones/us-east1-b", key)
				foundEast = true
			}
		}
	}
	assert.True(t, foundCentral, "us-central1-a disk must appear in aggregated list")
	assert.True(t, foundEast, "us-east1-b disk must appear in aggregated list")
}

func TestCompute_Disks_Get_NotFound(t *testing.T) {
	svc := computeService(t)
	_, err := svc.Disks.Get("test-project", "us-central1-a", "does-not-exist").Context(ctx).Do()
	require.Error(t, err, "get on missing disk must 404")
}

// TestCompute_GlobalHTTPLoadBalancerChain exercises the public Compute
// Engine global HTTP load-balancing API surface used by SDKs and
// Terraform: Insert/Get/List/Delete for HealthChecks, BackendServices,
// UrlMaps, TargetHttpProxies, and GlobalForwardingRules.
func TestCompute_GlobalHTTPLoadBalancerChain(t *testing.T) {
	svc := computeService(t)
	const project = "test-project"

	hc := &compute.HealthCheck{
		Name:             "sdk-lb-hc",
		Type:             "HTTP",
		CheckIntervalSec: 5,
		TimeoutSec:       5,
		HttpHealthCheck: &compute.HTTPHealthCheck{
			Port:        80,
			RequestPath: "/healthz",
		},
	}
	_, err := svc.HealthChecks.Insert(project, hc).Context(ctx).Do()
	require.NoError(t, err)
	gotHC, err := svc.HealthChecks.Get(project, hc.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "HTTP", gotHC.Type)
	assert.Equal(t, "/healthz", gotHC.HttpHealthCheck.RequestPath)

	bs := &compute.BackendService{
		Name:         "sdk-lb-backend",
		Protocol:     "HTTP",
		PortName:     "http",
		TimeoutSec:   10,
		HealthChecks: []string{gotHC.SelfLink},
	}
	_, err = svc.BackendServices.Insert(project, bs).Context(ctx).Do()
	require.NoError(t, err)
	gotBS, err := svc.BackendServices.Get(project, bs.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, []string{gotHC.SelfLink}, gotBS.HealthChecks)

	um := &compute.UrlMap{
		Name:           "sdk-lb-url-map",
		DefaultService: gotBS.SelfLink,
	}
	_, err = svc.UrlMaps.Insert(project, um).Context(ctx).Do()
	require.NoError(t, err)
	gotUM, err := svc.UrlMaps.Get(project, um.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, gotBS.SelfLink, gotUM.DefaultService)

	proxy := &compute.TargetHttpProxy{
		Name:   "sdk-lb-http-proxy",
		UrlMap: gotUM.SelfLink,
	}
	_, err = svc.TargetHttpProxies.Insert(project, proxy).Context(ctx).Do()
	require.NoError(t, err)
	gotProxy, err := svc.TargetHttpProxies.Get(project, proxy.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, gotUM.SelfLink, gotProxy.UrlMap)

	fr := &compute.ForwardingRule{
		Name:       "sdk-lb-fr",
		Target:     gotProxy.SelfLink,
		PortRange:  "80",
		IPProtocol: "TCP",
	}
	_, err = svc.GlobalForwardingRules.Insert(project, fr).Context(ctx).Do()
	require.NoError(t, err)
	gotFR, err := svc.GlobalForwardingRules.Get(project, fr.Name).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, gotProxy.SelfLink, gotFR.Target)
	assert.Equal(t, "80", gotFR.PortRange)
	assert.NotEmpty(t, gotFR.IPAddress)

	hcList, err := svc.HealthChecks.List(project).Context(ctx).Do()
	require.NoError(t, err)
	assert.NotEmpty(t, hcList.Items)
	frList, err := svc.GlobalForwardingRules.List(project).Context(ctx).Do()
	require.NoError(t, err)
	assert.NotEmpty(t, frList.Items)

	_, err = svc.GlobalForwardingRules.Delete(project, fr.Name).Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.TargetHttpProxies.Delete(project, proxy.Name).Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.UrlMaps.Delete(project, um.Name).Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.BackendServices.Delete(project, bs.Name).Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.HealthChecks.Delete(project, hc.Name).Context(ctx).Do()
	require.NoError(t, err)
}

func TestCompute_Instances_Lifecycle(t *testing.T) {
	svc := computeService(t)
	const project = "test-project"
	const zone = "us-central1-a"
	const region = "us-central1"

	network := &compute.Network{Name: "sdk-vm-network", AutoCreateSubnetworks: false}
	_, err := svc.Networks.Insert(project, network).Context(ctx).Do()
	require.NoError(t, err)
	subnet := &compute.Subnetwork{
		Name:        "sdk-vm-subnet",
		IpCidrRange: "10.26.0.0/24",
		Network:     "projects/test-project/global/networks/sdk-vm-network",
		Region:      region,
	}
	_, err = svc.Subnetworks.Insert(project, region, subnet).Context(ctx).Do()
	require.NoError(t, err)

	inst := &compute.Instance{
		Name:        "sdk-vm-1",
		MachineType: "e2-micro",
		Disks: []*compute.AttachedDisk{{
			Boot:       true,
			AutoDelete: true,
			InitializeParams: &compute.AttachedDiskInitializeParams{
				SourceImage: "projects/debian-cloud/global/images/debian-12",
				DiskSizeGb:  10,
			},
		}},
		NetworkInterfaces: []*compute.NetworkInterface{{
			Network:    "projects/test-project/global/networks/sdk-vm-network",
			Subnetwork: "projects/test-project/regions/us-central1/subnetworks/sdk-vm-subnet",
		}},
		Labels: map[string]string{"env": "sdk"},
		Tags:   &compute.Tags{Items: []string{"runner"}},
	}

	op, err := svc.Instances.Insert(project, zone, inst).Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "DONE", op.Status)

	got, err := svc.Instances.Get(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "sdk-vm-1", got.Name)
	assert.Equal(t, "RUNNING", got.Status)
	require.Len(t, got.NetworkInterfaces, 1)
	assert.NotEmpty(t, got.NetworkInterfaces[0].NetworkIP)

	list, err := svc.Instances.List(project, zone).Context(ctx).Do()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list.Items), 1)

	_, err = svc.Instances.Stop(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	stopped, err := svc.Instances.Get(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "TERMINATED", stopped.Status)

	_, err = svc.Instances.Start(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	running, err := svc.Instances.Get(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", running.Status)

	_, err = svc.Instances.Delete(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.NoError(t, err)
	_, err = svc.Instances.Get(project, zone, "sdk-vm-1").Context(ctx).Do()
	require.Error(t, err, "get after delete must fail")
}
