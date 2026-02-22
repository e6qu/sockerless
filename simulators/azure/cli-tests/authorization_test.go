package azure_cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const authAPIVersion = "2022-04-01"

func TestRoleAssignment_CreateAndShow(t *testing.T) {
	raName := "00000000-0000-0000-0000-000000000099"
	url := fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Authorization/roleAssignments/%s?api-version=%s",
		baseURL, subscriptionID, resourceGroup, raName, authAPIVersion)

	body := fmt.Sprintf(`{
		"properties": {
			"roleDefinitionId": "/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/acdd72a7-3385-48ef-bd42-f606fba81ae7",
			"principalId": "test-principal-id"
		}
	}`, subscriptionID)

	out := runCLI(t, azRest("PUT", url, body))

	var result struct {
		Name       string `json:"name"`
		Properties struct {
			RoleDefinitionId string `json:"roleDefinitionId"`
			PrincipalId      string `json:"principalId"`
		} `json:"properties"`
	}
	parseJSON(t, out, &result)
	assert.Equal(t, raName, result.Name)
	assert.Equal(t, "test-principal-id", result.Properties.PrincipalId)

	// GET
	out = runCLI(t, azRest("GET", url, ""))
	parseJSON(t, out, &result)
	assert.Equal(t, raName, result.Name)

	// Cleanup
	runCLI(t, azRest("DELETE", url, ""))
}

func TestRoleAssignment_Delete(t *testing.T) {
	raName := "00000000-0000-0000-0000-000000000098"
	url := fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Authorization/roleAssignments/%s?api-version=%s",
		baseURL, subscriptionID, resourceGroup, raName, authAPIVersion)

	body := fmt.Sprintf(`{
		"properties": {
			"roleDefinitionId": "/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/acdd72a7-3385-48ef-bd42-f606fba81ae7",
			"principalId": "delete-test-principal"
		}
	}`, subscriptionID)

	runCLI(t, azRest("PUT", url, body))
	runCLI(t, azRest("DELETE", url, ""))

	// Verify deletion
	cmd := azRest("GET", url, "")
	_, err := cmd.CombinedOutput()
	assert.Error(t, err, "Expected GET to fail after deletion")
}
