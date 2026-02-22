package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dnsAPIVersion = "2018-09-01"

func dnsURL(path string) string {
	return armURL("Microsoft.Network", path, dnsAPIVersion)
}

func TestPrivateDNS_CreateAndShowZone(t *testing.T) {
	url := dnsURL("privateDnsZones/cli-test.local")

	out := runCLI(t, azRest("PUT", url, `{"location":"global"}`))

	var zone struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	parseJSON(t, out, &zone)
	assert.Equal(t, "cli-test.local", zone.Name)
	assert.Equal(t, "global", zone.Location)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &zone)
	assert.Equal(t, "cli-test.local", zone.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
}

func TestPrivateDNS_CreateRecordSet(t *testing.T) {
	zoneURL := dnsURL("privateDnsZones/record-test.local")
	runCLI(t, azRest("PUT", zoneURL, `{"location":"global"}`))

	recordURL := dnsURL("privateDnsZones/record-test.local/A/myrecord")
	out := runCLI(t, azRest("PUT", recordURL,
		`{"properties":{"ttl":300,"aRecords":[{"ipv4Address":"10.0.0.1"}]}}`))

	var record struct {
		Name       string `json:"name"`
		Properties struct {
			TTL      int `json:"ttl"`
			ARecords []struct {
				IPv4Address string `json:"ipv4Address"`
			} `json:"aRecords"`
		} `json:"properties"`
	}
	parseJSON(t, out, &record)
	assert.Equal(t, "myrecord", record.Name)
	require.Len(t, record.Properties.ARecords, 1)
	assert.Equal(t, "10.0.0.1", record.Properties.ARecords[0].IPv4Address)

	// Cleanup
	runCLI(t, azRest("DELETE", recordURL, ""))
	runCLI(t, azRest("DELETE", zoneURL, ""))
}

func TestPrivateDNS_VNetLink(t *testing.T) {
	zoneURL := dnsURL("privateDnsZones/link-test.local")
	runCLI(t, azRest("PUT", zoneURL, `{"location":"global"}`))

	linkURL := dnsURL("privateDnsZones/link-test.local/virtualNetworkLinks/mylink")
	out := runCLI(t, azRest("PUT", linkURL,
		`{"location":"global","properties":{"virtualNetwork":{"id":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/myvnet"},"registrationEnabled":false}}`))

	var link struct {
		Name       string `json:"name"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	parseJSON(t, out, &link)
	assert.Equal(t, "mylink", link.Name)
	assert.Equal(t, "Succeeded", link.Properties.ProvisioningState)

	// GET
	out = runCLI(t, azRest("GET", linkURL, ""))
	parseJSON(t, out, &link)
	assert.Equal(t, "mylink", link.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", linkURL, ""))
	runCLI(t, azRest("DELETE", zoneURL, ""))
}

func TestPrivateDNS_DeleteZone(t *testing.T) {
	zoneURL := dnsURL("privateDnsZones/delete-test.local")
	runCLI(t, azRest("PUT", zoneURL, `{"location":"global"}`))
	runCLI(t, azRest("DELETE", zoneURL, ""))

	// Verify deletion - GET should fail
	cmd := azRest("GET", zoneURL, "")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err, "Expected GET to fail after deletion")
}
