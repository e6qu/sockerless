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

// TestACR_CacheRuleCRUD drives the `cacheRules` sub-resource
// through the `az rest` CLI. `az acr cache` commands are higher-level
// but route through the same ARM endpoints, so exercising via az rest
// covers both paths.
func TestACR_CacheRuleCRUD(t *testing.T) {
	regURL := acrURL("registries/cliregforcache")
	runCLI(t, azRest("PUT", regURL,
		`{"location":"eastus","sku":{"name":"Basic"},"properties":{}}`))
	defer runCLI(t, azRest("DELETE", regURL, ""))

	ruleURL := acrURL("registries/cliregforcache/cacheRules/docker-hub")

	// Create.
	body := `{"properties":{"sourceRepository":"docker.io/library/*","targetRepository":"docker-hub/library/*"}}`
	out := runCLI(t, azRest("PUT", ruleURL, body))
	var rule struct {
		Name       string `json:"name"`
		Properties struct {
			SourceRepository  string `json:"sourceRepository"`
			TargetRepository  string `json:"targetRepository"`
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	parseJSON(t, out, &rule)
	assert.Equal(t, "docker-hub", rule.Name)
	assert.Equal(t, "docker.io/library/*", rule.Properties.SourceRepository)
	assert.Equal(t, "docker-hub/library/*", rule.Properties.TargetRepository)
	assert.Equal(t, "Succeeded", rule.Properties.ProvisioningState)

	// Get.
	out = runCLI(t, azRest("GET", ruleURL, ""))
	parseJSON(t, out, &rule)
	assert.Equal(t, "docker-hub", rule.Name)

	// List.
	listURL := acrURL("registries/cliregforcache/cacheRules")
	out = runCLI(t, azRest("GET", listURL, ""))
	var listResp struct {
		Value []struct {
			Name string `json:"name"`
		} `json:"value"`
	}
	parseJSON(t, out, &listResp)
	var found bool
	for _, r := range listResp.Value {
		if r.Name == "docker-hub" {
			found = true
		}
	}
	assert.True(t, found, "expected List to return the created cache rule")

	// Delete.
	runCLI(t, azRest("DELETE", ruleURL, ""))
}
