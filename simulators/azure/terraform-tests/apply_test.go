package azure_tf_test

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformApplyDestroy provisions resource group, virtual network,
// subnet, NSG, and NSG security rule against the Azure simulator using
// the `azurestack` provider, then asserts canonical resource-id paths
// round-trip and terraform destroy cleans up.
//
// Slices exercised against the simulator:
//   - Microsoft.Resources/resourceGroups
//   - Microsoft.Network/virtualNetworks
//   - Microsoft.Network/virtualNetworks/subnets
//   - Microsoft.Network/networkSecurityGroups
//   - Microsoft.Network/networkSecurityGroups/securityRules
//
// The sibling `azurerm` cloud provider drives the same ARM endpoints.
func TestTerraformApplyDestroy(t *testing.T) {
	// The azurestack terraform provider (and the azurerm sibling)
	// validates the sim's self-signed HTTPS cert against the OS trust
	// store. On Linux it honours SSL_CERT_FILE (set by terraformCmd).
	// On darwin, Go's cgo-backed crypto/x509.SystemCertPool() reads
	// from the Security framework keychain and ignores SSL_CERT_FILE —
	// so terraform's outbound calls to the sim fail with `x509:
	// "localhost" certificate is not trusted`. We don't ship a darwin
	// workaround (would require sudo + persistent keychain trust).
	// Fail loud so the dev sees the limitation explicitly instead of
	// silently skipping. Run these tests in a Linux container or in CI.
	if runtime.GOOS == "darwin" {
		t.Fatal("darwin: SSL_CERT_FILE ignored by Go cgo SystemCertPool — terraform azurestack cannot validate the sim's self-signed cert. Run via a Linux Docker container or in CI.")
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
