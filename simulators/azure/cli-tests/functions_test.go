package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const functionsAPIVersion = "2022-09-01"

func funcURL(path string) string {
	return armURL("Microsoft.Web", path, functionsAPIVersion)
}

func TestFunctionApp_CreateAndShow(t *testing.T) {
	// Create an app service plan first
	planURL := aspURL("serverfarms/func-test-plan")
	runCLI(t, azRest("PUT", planURL, `{"location":"eastus","sku":{"name":"Y1","tier":"Dynamic"}}`))

	url := funcURL("sites/cli-test-funcapp")
	body := `{
		"location": "eastus",
		"kind": "functionapp",
		"properties": {
			"serverFarmId": "/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/cli-test-rg/providers/Microsoft.Web/serverfarms/func-test-plan",
			"siteConfig": {
				"appSettings": [
					{"name": "FUNCTIONS_EXTENSION_VERSION", "value": "~4"},
					{"name": "FUNCTIONS_WORKER_RUNTIME", "value": "node"}
				]
			}
		}
	}`

	out := runCLI(t, azRest("PUT", url, body))

	var site struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Kind     string `json:"kind"`
		Properties struct {
			State             string `json:"state"`
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	parseJSON(t, out, &site)
	assert.Equal(t, "cli-test-funcapp", site.Name)
	assert.Equal(t, "eastus", site.Location)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &site)
	assert.Equal(t, "cli-test-funcapp", site.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
	runCLI(t, azRest("DELETE", planURL, ""))
}

func TestFunctionApp_Delete(t *testing.T) {
	url := funcURL("sites/delete-test-funcapp")
	runCLI(t, azRest("PUT", url, `{"location":"eastus","properties":{}}`))
	runCLI(t, azRest("DELETE", url, ""))

	cmd := azRest("GET", url, "")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err, "Expected GET to fail after deletion")
}
