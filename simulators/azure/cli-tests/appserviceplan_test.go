package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const aspAPIVersion = "2022-09-01"

func aspURL(path string) string {
	return armURL("Microsoft.Web", path, aspAPIVersion)
}

func TestAppServicePlan_CreateAndShow(t *testing.T) {
	url := aspURL("serverfarms/cli-test-plan")

	out := runCLI(t, azRest("PUT", url,
		`{"location":"eastus","sku":{"name":"B1","tier":"Basic"}}`))

	var plan struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Sku      struct {
			Name string `json:"name"`
			Tier string `json:"tier"`
		} `json:"sku"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	parseJSON(t, out, &plan)
	assert.Equal(t, "cli-test-plan", plan.Name)
	assert.Equal(t, "eastus", plan.Location)
	assert.Equal(t, "B1", plan.Sku.Name)
	assert.Equal(t, "Succeeded", plan.Properties.ProvisioningState)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &plan)
	assert.Equal(t, "cli-test-plan", plan.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
}

func TestAppServicePlan_Delete(t *testing.T) {
	url := aspURL("serverfarms/delete-test-plan")
	runCLI(t, azRest("PUT", url, `{"location":"eastus"}`))
	runCLI(t, azRest("DELETE", url, ""))

	cmd := azRest("GET", url, "")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err, "Expected GET to fail after deletion")
}
