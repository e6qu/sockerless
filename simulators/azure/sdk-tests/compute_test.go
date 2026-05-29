package azure_sdk_test

import (
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v8"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompute_VirtualMachineLifecycle(t *testing.T) {
	const rg = "compute-rg"
	const vnetName = "compute-vnet"
	const subnetName = "compute-subnet"
	const nicName = "compute-nic"
	const vmName = "compute-vm"

	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	_, err = rgClient.CreateOrUpdate(ctx, rg, armresources.ResourceGroup{Location: to.Ptr("eastus")}, nil)
	require.NoError(t, err)

	vnetClient, err := armnetwork.NewVirtualNetworksClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	vnetPoller, err := vnetClient.BeginCreateOrUpdate(ctx, rg, vnetName, armnetwork.VirtualNetwork{
		Location: to.Ptr("eastus"),
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{
				AddressPrefixes: []*string{to.Ptr("10.90.0.0/16")},
			},
		},
	}, nil)
	require.NoError(t, err)
	_, err = vnetPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)

	subnetClient, err := armnetwork.NewSubnetsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	subnetPoller, err := subnetClient.BeginCreateOrUpdate(ctx, rg, vnetName, subnetName, armnetwork.Subnet{
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr("10.90.1.0/24"),
		},
	}, nil)
	require.NoError(t, err)
	subnetResp, err := subnetPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, subnetResp.ID)

	nicClient, err := armnetwork.NewInterfacesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	nicPoller, err := nicClient.BeginCreateOrUpdate(ctx, rg, nicName, armnetwork.Interface{
		Location: to.Ptr("eastus"),
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{{
				Name: to.Ptr("ipconfig1"),
				Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
					Subnet:                    &armnetwork.Subnet{ID: subnetResp.ID},
					PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
					PrivateIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
					Primary:                   to.Ptr(true),
				},
			}},
		},
	}, nil)
	require.NoError(t, err)
	nicResp, err := nicPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, nicResp.ID)

	vmClient, err := armcompute.NewVirtualMachinesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	sizeClient, err := armcompute.NewVirtualMachineSizesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	sizePager := sizeClient.NewListPager("eastus", nil)
	require.True(t, sizePager.More())
	sizePage, err := sizePager.NextPage(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, sizePage.Value)

	skuClient, err := armcompute.NewResourceSKUsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	skuPager := skuClient.NewListPager(nil)
	require.True(t, skuPager.More())
	skuPage, err := skuPager.NextPage(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, skuPage.Value)

	vmPoller, err := vmClient.BeginCreateOrUpdate(ctx, rg, vmName, armcompute.VirtualMachine{
		Location: to.Ptr("eastus"),
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypesStandardB1S),
			},
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: &armcompute.ImageReference{
					Publisher: to.Ptr("Canonical"),
					Offer:     to.Ptr("0001-com-ubuntu-server-jammy"),
					SKU:       to.Ptr("22_04-lts"),
					Version:   to.Ptr("latest"),
				},
				OSDisk: &armcompute.OSDisk{
					Name:         to.Ptr("compute-vm-osdisk"),
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
					Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
					DeleteOption: to.Ptr(armcompute.DiskDeleteOptionTypesDelete),
					DiskSizeGB:   to.Ptr[int32](30),
				},
			},
			OSProfile: &armcompute.OSProfile{
				ComputerName:  to.Ptr(vmName),
				AdminUsername: to.Ptr("azureuser"),
				AdminPassword: to.Ptr("Str0ng-password-12345!"),
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{{
					ID: nicResp.ID,
					Properties: &armcompute.NetworkInterfaceReferenceProperties{
						Primary: to.Ptr(true),
					},
				}},
			},
		},
		Tags: map[string]*string{"env": to.Ptr("sdk")},
	}, nil)
	require.NoError(t, err)
	vmResp, err := vmPoller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, vmResp.ID)
	assert.Equal(t, vmName, *vmResp.Name)

	got, err := vmClient.Get(ctx, rg, vmName, &armcompute.VirtualMachinesClientGetOptions{
		Expand: to.Ptr(armcompute.InstanceViewTypesInstanceView),
	})
	require.NoError(t, err)
	require.NotNil(t, got.Properties)
	require.NotNil(t, got.Properties.InstanceView)
	require.NotEmpty(t, got.Properties.InstanceView.Statuses)

	powerOff, err := vmClient.BeginPowerOff(ctx, rg, vmName, nil)
	require.NoError(t, err)
	_, err = powerOff.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	view, err := vmClient.InstanceView(ctx, rg, vmName, nil)
	require.NoError(t, err)
	assert.True(t, containsVMStatus(view.Statuses, "PowerState/stopped"))

	start, err := vmClient.BeginStart(ctx, rg, vmName, nil)
	require.NoError(t, err)
	_, err = start.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	view, err = vmClient.InstanceView(ctx, rg, vmName, nil)
	require.NoError(t, err)
	assert.True(t, containsVMStatus(view.Statuses, "PowerState/running"))

	del, err := vmClient.BeginDelete(ctx, rg, vmName, nil)
	require.NoError(t, err)
	_, err = del.PollUntilDone(ctx, nil)
	require.NoError(t, err)
}

func containsVMStatus(statuses []*armcompute.InstanceViewStatus, code string) bool {
	for _, status := range statuses {
		if status.Code != nil && strings.EqualFold(*status.Code, code) {
			return true
		}
	}
	return false
}
