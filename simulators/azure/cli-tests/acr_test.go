package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const acrAPIVersion = "2023-01-01-preview"

func acrURL(path string) string {
	return armURL("Microsoft.ContainerRegistry", path, acrAPIVersion)
}

func TestACR_CreateAndShow(t *testing.T) {
	url := acrURL("registries/clitestregistry")

	out := runCLI(t, azRest("PUT", url,
		`{"location":"eastus","sku":{"name":"Basic"},"properties":{"adminUserEnabled":false}}`))

	var registry struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Sku      struct {
			Name string `json:"name"`
		} `json:"sku"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
			LoginServer       string `json:"loginServer"`
		} `json:"properties"`
	}
	parseJSON(t, out, &registry)
	assert.Equal(t, "clitestregistry", registry.Name)
	assert.Equal(t, "eastus", registry.Location)
	assert.NotEmpty(t, registry.Properties.LoginServer)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &registry)
	assert.Equal(t, "clitestregistry", registry.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
}
