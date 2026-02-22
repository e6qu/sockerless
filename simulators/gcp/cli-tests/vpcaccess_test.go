package gcp_cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func vpcAccessURL(name string) string {
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/connectors/%s",
		baseURL, project, location, name)
}

func vpcAccessBaseURL() string {
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/connectors",
		baseURL, project, location)
}

func TestVPCAccess_CreateAndDescribe(t *testing.T) {
	url := vpcAccessBaseURL() + "?connectorId=cli-test-connector"
	out := httpDoJSON(t, "POST", url, `{
		"network": "default",
		"ipCidrRange": "10.8.0.0/28"
	}`)

	// Create returns an operation
	var op struct {
		Done bool `json:"done"`
	}
	parseJSON(t, out, &op)

	// GET the connector
	getURL := vpcAccessURL("cli-test-connector")
	out = httpDoJSON(t, "GET", getURL, "")

	var connector struct {
		Name        string `json:"name"`
		Network     string `json:"network"`
		IpCidrRange string `json:"ipCidrRange"`
		State       string `json:"state"`
	}
	parseJSON(t, out, &connector)
	assert.Contains(t, connector.Name, "cli-test-connector")
	assert.Equal(t, "default", connector.Network)
	assert.Equal(t, "10.8.0.0/28", connector.IpCidrRange)
	assert.Equal(t, "READY", connector.State)

	// Cleanup
	httpDoJSON(t, "DELETE", getURL, "")
}

func TestVPCAccess_List(t *testing.T) {
	url := vpcAccessBaseURL() + "?connectorId=list-test-connector"
	httpDoJSON(t, "POST", url, `{
		"network": "my-network",
		"ipCidrRange": "10.9.0.0/28"
	}`)

	// List connectors
	out := httpDoJSON(t, "GET", vpcAccessBaseURL(), "")

	var result struct {
		Connectors []struct {
			Name string `json:"name"`
		} `json:"connectors"`
	}
	parseJSON(t, out, &result)
	require.NotEmpty(t, result.Connectors)

	// Cleanup
	httpDoJSON(t, "DELETE", vpcAccessURL("list-test-connector"), "")
}

func TestVPCAccess_Delete(t *testing.T) {
	url := vpcAccessBaseURL() + "?connectorId=delete-test-connector"
	httpDoJSON(t, "POST", url, `{
		"network": "default",
		"ipCidrRange": "10.10.0.0/28"
	}`)

	// Delete
	httpDoJSON(t, "DELETE", vpcAccessURL("delete-test-connector"), "")

	// Verify gone
	resp, err := httpDo("GET", vpcAccessURL("delete-test-connector"), "")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}
