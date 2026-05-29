package azure_cli_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestComputeVirtualMachineLifecycleCLI(t *testing.T) {
	sizesURL := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.Compute/locations/eastus/vmSizes?api-version=2022-03-01", baseURL, subscriptionID)
	if out := runCLI(t, azRest("GET", sizesURL, "")); !strings.Contains(out, "Standard_B1s") {
		t.Fatalf("expected VM size discovery, got %s", out)
	}

	skusURL := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.Compute/skus?api-version=2021-07-01", baseURL, subscriptionID)
	if out := runCLI(t, azRest("GET", skusURL, "")); !strings.Contains(out, "Standard_B1s") {
		t.Fatalf("expected Compute SKU discovery, got %s", out)
	}

	vnetURL := armURL("Microsoft.Network", "virtualNetworks/cli-vm-vnet", "2024-05-01")
	runCLI(t, azRest("PUT", vnetURL, `{"location":"eastus","properties":{"addressSpace":{"addressPrefixes":["10.91.0.0/16"]}}}`))

	subnetID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/cli-vm-vnet/subnets/cli-vm-subnet", subscriptionID, resourceGroup)
	subnetURL := armURL("Microsoft.Network", "virtualNetworks/cli-vm-vnet/subnets/cli-vm-subnet", "2024-05-01")
	runCLI(t, azRest("PUT", subnetURL, `{"properties":{"addressPrefix":"10.91.1.0/24"}}`))

	nicID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/cli-vm-nic", subscriptionID, resourceGroup)
	nicURL := armURL("Microsoft.Network", "networkInterfaces/cli-vm-nic", "2024-05-01")
	nicBody := fmt.Sprintf(`{"location":"eastus","properties":{"ipConfigurations":[{"name":"ipconfig1","properties":{"subnet":{"id":%q},"privateIPAllocationMethod":"Dynamic","privateIPAddressVersion":"IPv4","primary":true}}]}}`, subnetID)
	runCLI(t, azRest("PUT", nicURL, nicBody))

	vmPath := fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/cli-vm", baseURL, subscriptionID, resourceGroup)
	vmURL := vmPath + "?api-version=2024-07-01"
	vmBody := fmt.Sprintf(`{"location":"eastus","properties":{"hardwareProfile":{"vmSize":"Standard_B1s"},"storageProfile":{"imageReference":{"publisher":"Canonical","offer":"0001-com-ubuntu-server-jammy","sku":"22_04-lts","version":"latest"},"osDisk":{"name":"cli-vm-osdisk","createOption":"FromImage","caching":"ReadWrite","deleteOption":"Delete","diskSizeGB":30}},"osProfile":{"computerName":"cli-vm","adminUsername":"azureuser","adminPassword":"Str0ng-password-12345!"},"networkProfile":{"networkInterfaces":[{"id":%q,"properties":{"primary":true}}]}}}`, nicID)
	runCLI(t, azRest("PUT", vmURL, vmBody))

	out := runCLI(t, azRest("GET", vmURL+"&$expand=instanceView", ""))
	if !strings.Contains(out, "PowerState/running") {
		t.Fatalf("expected running VM instance view, got %s", out)
	}

	runCLI(t, azRest("POST", vmPath+"/powerOff?api-version=2024-07-01", ""))
	out = runCLI(t, azRest("GET", vmPath+"/instanceView?api-version=2024-07-01", ""))
	if !strings.Contains(out, "PowerState/stopped") {
		t.Fatalf("expected stopped VM instance view, got %s", out)
	}

	runCLI(t, azRest("POST", vmPath+"/start?api-version=2024-07-01", ""))
	out = runCLI(t, azRest("GET", vmPath+"/instanceView?api-version=2024-07-01", ""))
	if !strings.Contains(out, "PowerState/running") {
		t.Fatalf("expected restarted VM instance view, got %s", out)
	}

	runCLI(t, azRest("DELETE", vmURL, ""))
}
