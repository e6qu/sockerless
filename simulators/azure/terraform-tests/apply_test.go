package azure_tf_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	realexec "github.com/sockerless/simulator-realexec"
	"github.com/stretchr/testify/require"
)

// TestTerraformApplyDestroy provisions a foundation set of Azure
// resources against the Azure simulator using the `azurerm` provider
// (the sim ships /metadata/endpoints + OAuth2 token endpoint + JWKS so
// azurerm can bootstrap its cloud config + auth without ever reaching
// real Azure). Then asserts canonical resource-id paths round-trip and
// terraform destroy cleans up.
//
// Slices exercised against the simulator:
//   - Microsoft.Resources/resourceGroups
//   - Microsoft.Network/virtualNetworks
//   - Microsoft.Network/virtualNetworks/subnets
//   - Microsoft.Network/networkSecurityGroups
//   - Microsoft.Network/networkSecurityGroups/securityRules
//   - Microsoft.Storage/storageAccounts
//   - Microsoft.KeyVault/vaults
//   - Microsoft.ContainerRegistry/registries
//   - Microsoft.Cache/Redis + firewallRules
//   - Microsoft.ManagedIdentity/userAssignedIdentities
//   - Microsoft.Network/publicIPAddresses + publicIPPrefixes + natGateways +
//     subnet NAT associations + loadBalancers
//   - Microsoft.Network/privateDnsZones
//   - Microsoft.Network/dnsZones + dnsZones/A
//   - Microsoft.ServiceBus/namespaces + queues
//   - Microsoft.EventGrid/topics
//   - Microsoft.DocumentDB/databaseAccounts + sqlDatabases + containers
//   - Microsoft.OperationalInsights/workspaces
//   - Microsoft.Insights/components
//   - Microsoft.App/managedEnvironments + containerApps + jobs
//   - Microsoft.Web/serverfarms + sites (Function App)
func TestTerraformApplyDestroy(t *testing.T) {
	requireTerraformNetworkHost(t)
	cleanTerraformWorkspace(t)
	out, err := runTimed(t, "terraform init", terraformCmd("init"))
	require.NoError(t, err, "terraform init failed:\n%s", out)

	out, err = runTimed(t, "terraform apply", terraformCmd("apply", "-auto-approve"))
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	// Idempotency: a second plan must show no drift. -detailed-exitcode makes
	// terraform exit 2 (non-zero) on any drift, which runTimed surfaces as an
	// error — so a clean plan (exit 0) is the only pass.
	out, err = runTimed(t, "terraform plan", terraformCmd("plan", "-detailed-exitcode"))
	require.NoError(t, err, "terraform plan showed drift after apply (not idempotent):\n%s", out)

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

	// Each canonical ARM path is asserted so a future provider/SDK
	// upgrade that mangles the URL surfaces in CI.
	azrmRG := outputs.must(t, "azrm_resource_group_id")
	require.True(t, strings.HasSuffix(azrmRG, "/resourceGroups/tf-azrm-rg"),
		"azurerm RG id must end with /resourceGroups/{name}; got %s", azrmRG)

	azrmACR := outputs.must(t, "azrm_acr_id")
	require.Contains(t, azrmACR, "/providers/Microsoft.ContainerRegistry/registries/tfazrmacr",
		"azurerm ACR id must include canonical ARM path; got %s", azrmACR)

	azrmRedisHost := outputs.must(t, "azrm_redis_cache_hostname")
	require.Contains(t, azrmRedisHost, "tfazrmredis.redis.cache.",
		"azurerm Redis hostname must include Azure Cache for Redis host shape; got %s", azrmRedisHost)

	azrmRedisFW := outputs.must(t, "azrm_redis_firewall_rule_id")
	require.Contains(t, strings.ToLower(azrmRedisFW), "/providers/microsoft.cache/redis/tfazrmredis/firewallrules/allow_ci",
		"azurerm Redis firewall rule id must include canonical ARM path; got %s", azrmRedisFW)

	azrmUAI := outputs.must(t, "azrm_uai_id")
	require.Contains(t, azrmUAI, "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/tf-azrm-uai",
		"azurerm managed identity id must include canonical ARM path; got %s", azrmUAI)

	azrmLB := outputs.must(t, "azrm_lb_id")
	require.Contains(t, azrmLB, "/providers/Microsoft.Network/loadBalancers/tf-azrm-lb",
		"azurerm Load Balancer id must include canonical ARM path; got %s", azrmLB)

	azrmLBBackend := outputs.must(t, "azrm_lb_backend_pool_id")
	require.Contains(t, azrmLBBackend, "/loadBalancers/tf-azrm-lb/backendAddressPools/backend",
		"azurerm Load Balancer backend pool id must include child ARM path; got %s", azrmLBBackend)

	azrmLBProbe := outputs.must(t, "azrm_lb_probe_id")
	require.Contains(t, azrmLBProbe, "/loadBalancers/tf-azrm-lb/probes/tcp-probe",
		"azurerm Load Balancer probe id must include child ARM path; got %s", azrmLBProbe)

	azrmLBRule := outputs.must(t, "azrm_lb_rule_id")
	require.Contains(t, azrmLBRule, "/loadBalancers/tf-azrm-lb/loadBalancingRules/http-rule",
		"azurerm Load Balancer rule id must include child ARM path; got %s", azrmLBRule)

	azrmNAT := outputs.must(t, "azrm_nat_gateway_id")
	require.Contains(t, azrmNAT, "/providers/Microsoft.Network/natGateways/tf-azrm-nat",
		"azurerm NAT Gateway id must include canonical ARM path; got %s", azrmNAT)

	azrmNATPrefix := outputs.must(t, "azrm_nat_public_ip_prefix_id")
	require.Contains(t, azrmNATPrefix, "/providers/Microsoft.Network/publicIPPrefixes/tf-azrm-nat-prefix",
		"azurerm Public IP Prefix id must include canonical ARM path; got %s", azrmNATPrefix)

	azrmNATSubnet := outputs.must(t, "azrm_nat_subnet_id")
	require.Contains(t, azrmNATSubnet, "/virtualNetworks/tf-azrm-nat-vnet/subnets/tf-azrm-nat-subnet",
		"azurerm subnet NAT association must keep the associated subnet id; got %s", azrmNATSubnet)

	azrmDNS := outputs.must(t, "azrm_private_dns_zone_id")
	require.Contains(t, azrmDNS, "/providers/Microsoft.Network/privateDnsZones/tf-azrm.internal",
		"azurerm private DNS zone id must include canonical ARM path; got %s", azrmDNS)

	azrmPublicDNS := outputs.must(t, "azrm_public_dns_zone_id")
	require.Contains(t, azrmPublicDNS, "/providers/Microsoft.Network/dnsZones/tf-azrm.example.com",
		"azurerm public DNS zone id must include canonical ARM path; got %s", azrmPublicDNS)

	azrmPublicDNSA := outputs.must(t, "azrm_public_dns_a_record_id")
	require.Contains(t, azrmPublicDNSA, "/providers/Microsoft.Network/dnsZones/tf-azrm.example.com/A/www",
		"azurerm public DNS A record id must include canonical ARM path; got %s", azrmPublicDNSA)

	azrmServiceBusNamespace := outputs.must(t, "azrm_servicebus_namespace_id")
	require.Contains(t, azrmServiceBusNamespace, "/providers/Microsoft.ServiceBus/namespaces/tfazrmsbns",
		"azurerm Service Bus namespace id must include canonical ARM path; got %s", azrmServiceBusNamespace)

	azrmServiceBusQueue := outputs.must(t, "azrm_servicebus_queue_id")
	require.Contains(t, azrmServiceBusQueue, "/providers/Microsoft.ServiceBus/namespaces/tfazrmsbns/queues/tfazrmsbqueue",
		"azurerm Service Bus queue id must include canonical ARM path; got %s", azrmServiceBusQueue)

	azrmEventGridEndpoint := outputs.must(t, "azrm_eventgrid_topic_endpoint")
	require.Contains(t, azrmEventGridEndpoint, "/api/events",
		"azurerm Event Grid topic endpoint must be a publish endpoint; got %s", azrmEventGridEndpoint)

	azrmCosmosEndpoint := outputs.must(t, "azrm_cosmosdb_account_endpoint")
	require.Contains(t, azrmCosmosEndpoint, "tfazrmcosmos.documents.",
		"azurerm Cosmos DB endpoint must include account documents host; got %s", azrmCosmosEndpoint)

	azrmCosmosContainer := outputs.must(t, "azrm_cosmosdb_sql_container_id")
	require.Contains(t, azrmCosmosContainer, "/providers/Microsoft.DocumentDB/databaseAccounts/tfazrmcosmos/sqlDatabases/tfappdb/containers/users",
		"azurerm Cosmos SQL container id must include canonical ARM path; got %s", azrmCosmosContainer)

	azrmCosmosTable := outputs.must(t, "azrm_cosmosdb_table_id")
	require.Contains(t, azrmCosmosTable, "/providers/Microsoft.DocumentDB/databaseAccounts/tfazrmcosmos/tables/tfcosmostable",
		"azurerm Cosmos table id must include canonical ARM path; got %s", azrmCosmosTable)

	azrmKVPolicy := outputs.must(t, "azrm_key_vault_access_policy_id")
	require.Contains(t, strings.ToLower(azrmKVPolicy), "/providers/microsoft.keyvault/vaults/tf-azrm-kv",
		"azurerm Key Vault access policy id must include vault ARM path; got %s", azrmKVPolicy)

	azrmKVKey := outputs.must(t, "azrm_key_vault_key_id")
	require.Contains(t, azrmKVKey, "tf-azrm-kv.vault.",
		"azurerm Key Vault key id must be data-plane URL-shaped; got %s", azrmKVKey)
	require.Contains(t, azrmKVKey, "/keys/tf-azrm-key/",
		"azurerm Key Vault key id must include key name and version; got %s", azrmKVKey)

	azrmKVCert := outputs.must(t, "azrm_key_vault_certificate_id")
	require.Contains(t, azrmKVCert, "tf-azrm-kv.vault.",
		"azurerm Key Vault certificate id must be data-plane URL-shaped; got %s", azrmKVCert)
	require.Contains(t, azrmKVCert, "/certificates/tf-azrm-cert/",
		"azurerm Key Vault certificate id must include certificate name and version; got %s", azrmKVCert)

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

	azrmLogic := outputs.must(t, "azrm_logic_app_workflow_id")
	require.Contains(t, azrmLogic, "/providers/Microsoft.Logic/workflows/tf-azrm-logic",
		"azurerm Logic App workflow id must include canonical ARM path; got %s", azrmLogic)

	azrmACI := outputs.must(t, "azrm_container_group_id")
	require.Contains(t, azrmACI, "/providers/Microsoft.ContainerInstance/containerGroups/tf-azrm-aci",
		"azurerm container group id must include canonical ARM path; got %s", azrmACI)

	azrmSP := outputs.must(t, "azrm_service_plan_id")
	// terraform-provider-azurerm normalizes Microsoft.Web/serverfarms
	// to the SDK-canonical `serverFarms` (camelCase) in state. The
	// sim emits lowercase in its ARM responses; the provider's ID
	// parser uppercases the F before storing. Match the provider's
	// canonical form here.
	require.Contains(t, strings.ToLower(azrmSP), "/providers/microsoft.web/serverfarms/tf-azrm-sp",
		"azurerm Service Plan id must include canonical ARM path (case-insensitive); got %s", azrmSP)

	azrmST := outputs.must(t, "azrm_storage_account_id")
	require.Contains(t, azrmST, "/providers/Microsoft.Storage/storageAccounts/tfazrmst12345",
		"azurerm storage account id must include canonical ARM path; got %s", azrmST)

	azrmStorageContainer := outputs.must(t, "azrm_storage_container_id")
	require.Contains(t, azrmStorageContainer, "tfazrmst12345.blob.",
		"azurerm storage container id must be data-plane blob URL-shaped; got %s", azrmStorageContainer)
	require.Contains(t, azrmStorageContainer, "/tfazrmcontainer",
		"azurerm storage container id must include container name; got %s", azrmStorageContainer)

	azrmStorageTable := outputs.must(t, "azrm_storage_table_id")
	require.Contains(t, azrmStorageTable, "tfazrmst12345.table.",
		"azurerm storage table id must be data-plane table URL-shaped; got %s", azrmStorageTable)
	require.Contains(t, azrmStorageTable, "/Tables('tfazrmstable')",
		"azurerm storage table id must include table name; got %s", azrmStorageTable)

	azrmFA := outputs.must(t, "azrm_function_app_id")
	require.Contains(t, azrmFA, "/providers/Microsoft.Web/sites/tf-azrm-fa",
		"azurerm Function App id must include canonical ARM path; got %s", azrmFA)

	azrmAPIM := outputs.must(t, "azrm_apim_id")
	require.Contains(t, azrmAPIM, "/providers/Microsoft.ApiManagement/service/tf-azrm-apim",
		"azurerm API Management id must include canonical ARM path; got %s", azrmAPIM)

	azrmAPIMGateway := outputs.must(t, "azrm_apim_gateway_url")
	require.Contains(t, azrmAPIMGateway, "tf-azrm-apim",
		"APIM gateway_url must reference the service name (round-trips properties.gatewayUrl); got %s", azrmAPIMGateway)

	azrmAPIMApi := outputs.must(t, "azrm_apim_api_id")
	require.Contains(t, azrmAPIMApi, "/service/tf-azrm-apim/apis/tf-azrm-api",
		"APIM API id must include the service+api path; got %s", azrmAPIMApi)

	azrmAPIMProduct := outputs.must(t, "azrm_apim_product_id")
	require.Contains(t, azrmAPIMProduct, "/service/tf-azrm-apim/products/tf-azrm-product",
		"APIM product id must include the service+product path; got %s", azrmAPIMProduct)

	azrmAPIMSub := outputs.must(t, "azrm_apim_subscription_id")
	require.Contains(t, azrmAPIMSub, "/service/tf-azrm-apim/subscriptions/",
		"APIM subscription id must include the service+subscription path; got %s", azrmAPIMSub)

	out, err = runTimed(t, "terraform destroy", terraformCmd("destroy", "-auto-approve"))
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

func requireTerraformNetworkHost(t *testing.T) {
	t.Helper()
	if err := realexec.DetectNetworkCapabilities().Require(); err != nil {
		t.Skipf("skipping: Terraform Network coverage requires host capabilities the simulator cannot provide here: %v", err)
	}
}

func cleanTerraformWorkspace(t *testing.T) {
	t.Helper()
	dir := filepath.Dir(mustAbs("main.tf"))
	for _, name := range []string{
		".terraform",
		".terraform.lock.hcl",
		"terraform.tfstate",
		"terraform.tfstate.backup",
		"crash.log",
	} {
		err := os.RemoveAll(filepath.Join(dir, name))
		require.NoError(t, err, "clean terraform workspace artifact %s", name)
	}
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
