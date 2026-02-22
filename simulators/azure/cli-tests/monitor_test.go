package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const monitorAPIVersion = "2022-10-01"

func monitorURL(path string) string {
	return armURL("Microsoft.OperationalInsights", path, monitorAPIVersion)
}

func TestMonitorWorkspace_CreateAndShow(t *testing.T) {
	url := monitorURL("workspaces/cli-test-workspace")

	out := runCLI(t, azRest("PUT", url,
		`{"location":"eastus","properties":{"retentionInDays":30}}`))

	var ws struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
			WorkspaceId       string `json:"customerId"`
		} `json:"properties"`
	}
	parseJSON(t, out, &ws)
	assert.Equal(t, "cli-test-workspace", ws.Name)
	assert.Equal(t, "eastus", ws.Location)
	assert.Equal(t, "Succeeded", ws.Properties.ProvisioningState)
	assert.NotEmpty(t, ws.Properties.WorkspaceId)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &ws)
	assert.Equal(t, "cli-test-workspace", ws.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
}

func TestMonitorWorkspace_Delete(t *testing.T) {
	url := monitorURL("workspaces/delete-test-ws")
	runCLI(t, azRest("PUT", url, `{"location":"eastus","properties":{}}`))
	runCLI(t, azRest("DELETE", url, ""))

	cmd := azRest("GET", url, "")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err, "Expected GET to fail after deletion")
}
