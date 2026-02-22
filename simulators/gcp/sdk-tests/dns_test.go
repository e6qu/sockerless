package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

func dnsService(t *testing.T) *dns.Service {
	t.Helper()
	svc, err := dns.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func TestDNS_CreateManagedZone(t *testing.T) {
	svc := dnsService(t)

	zone := &dns.ManagedZone{
		Name:    "test-zone",
		DnsName: "test.example.com.",
	}
	created, err := svc.ManagedZones.Create("test-project", zone).Do()
	require.NoError(t, err)
	assert.Equal(t, "test-zone", created.Name)
	assert.Equal(t, "test.example.com.", created.DnsName)
}

func TestDNS_GetManagedZone(t *testing.T) {
	svc := dnsService(t)

	zone := &dns.ManagedZone{
		Name:    "get-zone",
		DnsName: "get.example.com.",
	}
	_, err := svc.ManagedZones.Create("test-project", zone).Do()
	require.NoError(t, err)

	got, err := svc.ManagedZones.Get("test-project", "get-zone").Do()
	require.NoError(t, err)
	assert.Equal(t, "get-zone", got.Name)
}

func TestDNS_ListManagedZones(t *testing.T) {
	svc := dnsService(t)

	zone := &dns.ManagedZone{
		Name:    "list-zone",
		DnsName: "list.example.com.",
	}
	_, err := svc.ManagedZones.Create("test-project", zone).Do()
	require.NoError(t, err)

	resp, err := svc.ManagedZones.List("test-project").Do()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.ManagedZones), 1)
}

func TestDNS_DeleteManagedZone(t *testing.T) {
	svc := dnsService(t)

	zone := &dns.ManagedZone{
		Name:    "del-zone",
		DnsName: "del.example.com.",
	}
	_, err := svc.ManagedZones.Create("test-project", zone).Do()
	require.NoError(t, err)

	err = svc.ManagedZones.Delete("test-project", "del-zone").Do()
	require.NoError(t, err)
}
