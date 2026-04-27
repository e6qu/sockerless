package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetwork_CreateVirtualNetwork(t *testing.T) {
	// Create resource group first
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "net-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	client, err := armnetwork.NewVirtualNetworksClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	poller, err := client.BeginCreateOrUpdate(ctx, "net-rg", "test-vnet", armnetwork.VirtualNetwork{
		Location: ptrStr("eastus"),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{
				AddressPrefixes: []*string{ptrStr("10.0.0.0/16")},
			},
		},
	}, nil)
	require.NoError(t, err)
	resp, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-vnet", *resp.Name)
}

func TestNetwork_CreateSubnet(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "subnet-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	vnetClient, err := armnetwork.NewVirtualNetworksClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	vnetPoller, err := vnetClient.BeginCreateOrUpdate(ctx, "subnet-rg", "subnet-vnet", armnetwork.VirtualNetwork{
		Location: ptrStr("eastus"),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{
				AddressPrefixes: []*string{ptrStr("10.1.0.0/16")},
			},
		},
	}, nil)
	require.NoError(t, err)
	_, err = vnetPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	subnetClient, err := armnetwork.NewSubnetsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	subnetPoller, err := subnetClient.BeginCreateOrUpdate(ctx, "subnet-rg", "subnet-vnet", "test-subnet", armnetwork.Subnet{
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: ptrStr("10.1.1.0/24"),
		},
	}, nil)
	require.NoError(t, err)
	subnetResp, err := subnetPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-subnet", *subnetResp.Name)
}

func TestNetwork_CreateNSG(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "nsg-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	client, err := armnetwork.NewSecurityGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	poller, err := client.BeginCreateOrUpdate(ctx, "nsg-rg", "test-nsg", armnetwork.SecurityGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)
	resp, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-nsg", *resp.Name)
}

// TestNetwork_NSGSecurityRulesCRUD covers the `securityRules` sub-
// resource endpoints. Creates an NSG, then creates/gets/lists/deletes
// a security rule via the per-rule client, and confirms the rule is
// reflected in the parent NSG's properties.
func TestNetwork_NSGSecurityRulesCRUD(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "nsg-rules-rg", armresources.ResourceGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)

	nsgClient, err := armnetwork.NewSecurityGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	poller, err := nsgClient.BeginCreateOrUpdate(ctx, "nsg-rules-rg", "rules-nsg", armnetwork.SecurityGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)
	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	rulesClient, err := armnetwork.NewSecurityRulesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	// Create rule
	rulePoller, err := rulesClient.BeginCreateOrUpdate(ctx, "nsg-rules-rg", "rules-nsg", "allow-http",
		armnetwork.SecurityRule{
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Protocol:                 ptrProto(armnetwork.SecurityRuleProtocolTCP),
				SourcePortRange:          ptrStr("*"),
				DestinationPortRange:     ptrStr("80"),
				SourceAddressPrefix:      ptrStr("*"),
				DestinationAddressPrefix: ptrStr("*"),
				Access:                   ptrAccess(armnetwork.SecurityRuleAccessAllow),
				Priority:                 ptrInt32(100),
				Direction:                ptrDir(armnetwork.SecurityRuleDirectionInbound),
			},
		}, nil)
	require.NoError(t, err)
	ruleResp, err := rulePoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "allow-http", *ruleResp.Name)
	require.NotNil(t, ruleResp.Properties)
	assert.Equal(t, int32(100), *ruleResp.Properties.Priority)

	// Get rule
	getResp, err := rulesClient.Get(ctx, "nsg-rules-rg", "rules-nsg", "allow-http", nil)
	require.NoError(t, err)
	assert.Equal(t, "allow-http", *getResp.Name)

	// Parent NSG Get should now include the rule in its Properties.
	nsgResp, err := nsgClient.Get(ctx, "nsg-rules-rg", "rules-nsg", nil)
	require.NoError(t, err)
	require.NotNil(t, nsgResp.Properties)
	require.Len(t, nsgResp.Properties.SecurityRules, 1)
	assert.Equal(t, "allow-http", *nsgResp.Properties.SecurityRules[0].Name)

	// Delete rule
	delPoller, err := rulesClient.BeginDelete(ctx, "nsg-rules-rg", "rules-nsg", "allow-http", nil)
	require.NoError(t, err)
	_, err = delPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Parent NSG should no longer have the rule.
	nsgResp, err = nsgClient.Get(ctx, "nsg-rules-rg", "rules-nsg", nil)
	require.NoError(t, err)
	assert.Empty(t, nsgResp.Properties.SecurityRules)
}

// TestNetwork_NSGRule_RejectsDuplicatePriority pins the priority+
// direction uniqueness constraint that real Azure enforces. Two rules
// in the same NSG can share a priority only when they differ in
// Direction; otherwise the second create returns
// SecurityRuleParameterPriorityAlreadyTaken (HTTP 400).
func TestNetwork_NSGRule_RejectsDuplicatePriority(t *testing.T) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, "nsg-prio-rg", armresources.ResourceGroup{Location: ptrStr("eastus")}, nil)
	require.NoError(t, err)

	nsgClient, err := armnetwork.NewSecurityGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	poller, err := nsgClient.BeginCreateOrUpdate(ctx, "nsg-prio-rg", "prio-nsg", armnetwork.SecurityGroup{
		Location: ptrStr("eastus"),
	}, nil)
	require.NoError(t, err)
	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	rulesClient, err := armnetwork.NewSecurityRulesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	makeRule := func(priority int32, direction armnetwork.SecurityRuleDirection) armnetwork.SecurityRule {
		return armnetwork.SecurityRule{
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Protocol:                 ptrProto(armnetwork.SecurityRuleProtocolTCP),
				SourcePortRange:          ptrStr("*"),
				DestinationPortRange:     ptrStr("80"),
				SourceAddressPrefix:      ptrStr("*"),
				DestinationAddressPrefix: ptrStr("*"),
				Access:                   ptrAccess(armnetwork.SecurityRuleAccessAllow),
				Priority:                 ptrInt32(priority),
				Direction:                ptrDir(direction),
			},
		}
	}

	// First rule: priority 200, INGRESS — should succeed.
	p1, err := rulesClient.BeginCreateOrUpdate(ctx, "nsg-prio-rg", "prio-nsg", "first",
		makeRule(200, armnetwork.SecurityRuleDirectionInbound), nil)
	require.NoError(t, err)
	_, err = p1.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	// Second rule: same priority + direction — must fail.
	_, err = rulesClient.BeginCreateOrUpdate(ctx, "nsg-prio-rg", "prio-nsg", "duplicate",
		makeRule(200, armnetwork.SecurityRuleDirectionInbound), nil)
	require.Error(t, err, "duplicate Priority+Direction must fail like real Azure")

	// Third rule: same priority but EGRESS — should succeed (priority
	// space is per-direction, not global).
	p3, err := rulesClient.BeginCreateOrUpdate(ctx, "nsg-prio-rg", "prio-nsg", "egress-same-prio",
		makeRule(200, armnetwork.SecurityRuleDirectionOutbound), nil)
	require.NoError(t, err)
	_, err = p3.PollUntilDone(ctx, nil)
	require.NoError(t, err, "same priority across different directions must be accepted")
}

func ptrProto(p armnetwork.SecurityRuleProtocol) *armnetwork.SecurityRuleProtocol { return &p }
func ptrAccess(a armnetwork.SecurityRuleAccess) *armnetwork.SecurityRuleAccess    { return &a }
func ptrDir(d armnetwork.SecurityRuleDirection) *armnetwork.SecurityRuleDirection { return &d }
func ptrInt32(v int32) *int32                                                     { return &v }
