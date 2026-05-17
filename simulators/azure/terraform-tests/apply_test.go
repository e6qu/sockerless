package azure_tf_test

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformApplyDestroy provisions a foundation set of Azure
// resources against the Azure simulator using both the `azurestack`
// provider (for network primitives + storage + key vault control plane)
// and the `azurerm` provider (for ACR, Container Apps, Function App,
// Application Insights, managed identity, private DNS — surfaces the
// azurestack provider catalogue doesn't expose). Then asserts canonical
// resource-id paths round-trip and terraform destroy cleans up.
//
// Slices exercised against the simulator (azurestack):
//   - Microsoft.Resources/resourceGroups
//   - Microsoft.Network/virtualNetworks
//   - Microsoft.Network/virtualNetworks/subnets
//   - Microsoft.Network/networkSecurityGroups
//   - Microsoft.Network/networkSecurityGroups/securityRules
//   - Microsoft.Storage/storageAccounts
//   - Microsoft.KeyVault/vaults
//
// Slices exercised via azurerm (sim ships /metadata/endpoints + OAuth2
// token endpoint + JWKS so azurerm can bootstrap its cloud config + auth
// without ever reaching real Azure):
//   - Microsoft.ContainerRegistry/registries
//   - Microsoft.ManagedIdentity/userAssignedIdentities
//   - Microsoft.Network/privateDnsZones
//   - Microsoft.OperationalInsights/workspaces
//   - Microsoft.Insights/components
//   - Microsoft.App/managedEnvironments + containerApps + jobs
//   - Microsoft.Web/serverfarms + sites (Function App)
//   - Microsoft.Storage/storageAccounts (azurerm-managed)
func TestTerraformApplyDestroy(t *testing.T) {
	// The azurestack + azurerm terraform providers validate the sim's
	// self-signed HTTPS cert against the OS trust store. On Linux they
	// honour SSL_CERT_FILE (set by terraformCmd). On darwin, Go's
	// cgo-backed crypto/x509.SystemCertPool() reads from the Security
	// framework keychain and ignores SSL_CERT_FILE; GODEBUG=
	// x509usefallbackroots=1 doesn't bridge it (it only kicks in when
	// the platform pool comes back empty, not to *supplement* it).
	// Fail loud so the dev sees the limitation; run via CI or a Linux
	// container.
	if runtime.GOOS == "darwin" {
		t.Fatal("darwin: SSL_CERT_FILE ignored by Go cgo SystemCertPool — terraform azurestack/azurerm cannot validate the sim's self-signed cert. Run via Docker or in CI.")
	}

	init := terraformCmd("init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	outputs := readOutputs(t)

	rgID := outputs.must(t, "resource_group_id")
	require.True(t, strings.HasSuffix(rgID, "/resourceGroups/tf-test-rg"),
		"resource group id must end with /resourceGroups/{name}; got %s", rgID)

	vnetID := outputs.must(t, "vnet_id")
	require.Contains(t, vnetID, "/resourceGroups/tf-test-rg/providers/Microsoft.Network/virtualNetworks/tf-test-vnet",
		"vnet id must include the canonical ARM path; got %s", vnetID)

	subnetID := outputs.must(t, "subnet_id")
	require.Contains(t, subnetID, "/virtualNetworks/tf-test-vnet/subnets/tf-test-subnet",
		"subnet id must include the canonical ARM path; got %s", subnetID)

	nsgID := outputs.must(t, "nsg_id")
	require.Contains(t, nsgID, "/networkSecurityGroups/tf-test-nsg",
		"nsg id must include the canonical ARM path; got %s", nsgID)

	nsgRuleID := outputs.must(t, "nsg_rule_id")
	require.Contains(t, nsgRuleID, "/networkSecurityGroups/tf-test-nsg/securityRules/allow-ssh",
		"nsg rule id must include the canonical ARM path; got %s", nsgRuleID)

	storageID := outputs.must(t, "storage_account_id")
	require.Contains(t, storageID, "/providers/Microsoft.Storage/storageAccounts/tftestsa12345",
		"storage account id must include the canonical ARM path; got %s", storageID)

	blobEndpoint := outputs.must(t, "storage_account_blob_endpoint")
	require.True(t, strings.Contains(blobEndpoint, "tftestsa12345.blob."),
		"blob endpoint must include account subdomain (azurerm storage SDK parses URLs this way); got %s", blobEndpoint)

	kvID := outputs.must(t, "key_vault_id")
	require.Contains(t, kvID, "/providers/Microsoft.KeyVault/vaults/tf-test-kv",
		"key vault id must include the canonical ARM path; got %s", kvID)

	kvURI := outputs.must(t, "key_vault_uri")
	require.True(t, strings.Contains(kvURI, "tf-test-kv.vault."),
		"vault uri must include vault subdomain (azurerm/keyvault SDK parses URLs this way); got %s", kvURI)

	// azurerm-driven resources — provider routes via custom cloud
	// metadata + OAuth2 token endpoint exposed by the sim. Each
	// canonical ARM path is asserted so a future provider/SDK upgrade
	// that mangles the URL surfaces in CI.
	azrmRG := outputs.must(t, "azrm_resource_group_id")
	require.True(t, strings.HasSuffix(azrmRG, "/resourceGroups/tf-azrm-rg"),
		"azurerm RG id must end with /resourceGroups/{name}; got %s", azrmRG)

	azrmACR := outputs.must(t, "azrm_acr_id")
	require.Contains(t, azrmACR, "/providers/Microsoft.ContainerRegistry/registries/tfazrmacr",
		"azurerm ACR id must include canonical ARM path; got %s", azrmACR)

	azrmUAI := outputs.must(t, "azrm_uai_id")
	require.Contains(t, azrmUAI, "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/tf-azrm-uai",
		"azurerm managed identity id must include canonical ARM path; got %s", azrmUAI)

	azrmDNS := outputs.must(t, "azrm_private_dns_zone_id")
	require.Contains(t, azrmDNS, "/providers/Microsoft.Network/privateDnsZones/tf-azrm.internal",
		"azurerm private DNS zone id must include canonical ARM path; got %s", azrmDNS)

	azrmLAW := outputs.must(t, "azrm_law_id")
	require.Contains(t, azrmLAW, "/providers/Microsoft.OperationalInsights/workspaces/tf-azrm-law",
		"azurerm Log Analytics workspace id must include canonical ARM path; got %s", azrmLAW)

	azrmAI := outputs.must(t, "azrm_appins_id")
	require.Contains(t, azrmAI, "/providers/Microsoft.Insights/components/tf-azrm-ai",
		"azurerm Application Insights id must include canonical ARM path; got %s", azrmAI)

	azrmCAE := outputs.must(t, "azrm_container_app_env_id")
	require.Contains(t, azrmCAE, "/providers/Microsoft.App/managedEnvironments/tf-azrm-cae",
		"azurerm Container App Environment id must include canonical ARM path; got %s", azrmCAE)

	azrmCA := outputs.must(t, "azrm_container_app_id")
	require.Contains(t, azrmCA, "/providers/Microsoft.App/containerApps/tf-azrm-ca",
		"azurerm Container App id must include canonical ARM path; got %s", azrmCA)

	azrmCAJ := outputs.must(t, "azrm_container_app_job_id")
	require.Contains(t, azrmCAJ, "/providers/Microsoft.App/jobs/tf-azrm-caj",
		"azurerm Container App Job id must include canonical ARM path; got %s", azrmCAJ)

	azrmSP := outputs.must(t, "azrm_service_plan_id")
	require.Contains(t, azrmSP, "/providers/Microsoft.Web/serverfarms/tf-azrm-sp",
		"azurerm Service Plan id must include canonical ARM path; got %s", azrmSP)

	azrmST := outputs.must(t, "azrm_storage_account_id")
	require.Contains(t, azrmST, "/providers/Microsoft.Storage/storageAccounts/tfazrmst12345",
		"azurerm storage account id must include canonical ARM path; got %s", azrmST)

	azrmFA := outputs.must(t, "azrm_function_app_id")
	require.Contains(t, azrmFA, "/providers/Microsoft.Web/sites/tf-azrm-fa",
		"azurerm Function App id must include canonical ARM path; got %s", azrmFA)

	destroy := terraformCmd("destroy", "-auto-approve")
	out, err = destroy.CombinedOutput()
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

type tfOutputs map[string]struct {
	Sensitive bool        `json:"sensitive"`
	Type      interface{} `json:"type"`
	Value     interface{} `json:"value"`
}

func (o tfOutputs) must(t *testing.T, key string) string {
	t.Helper()
	v, ok := o[key]
	require.True(t, ok, "output %q missing from terraform state", key)
	s, ok := v.Value.(string)
	require.True(t, ok, "output %q is not a string (got %T)", key, v.Value)
	require.NotEmpty(t, s, "output %q is empty", key)
	return s
}

func readOutputs(t *testing.T) tfOutputs {
	t.Helper()
	out, err := terraformCmd("output", "-json").CombinedOutput()
	require.NoError(t, err, "terraform output failed:\n%s", out)
	var outputs tfOutputs
	require.NoError(t, json.Unmarshal(out, &outputs))
	return outputs
}
