# Sim surface — azure-compute

Surface registered in `simulators/azure/compute.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | cli-test | tf-test | notes |
|---|---|---|---|---|---|
| `PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/publicIPAddresses/{publicIPName}` | ✓ `simulators/azure/compute.go::registerPublicIPAddresses` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | n/a | Public IP control plane for VM/NIC references. |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/publicIPAddresses/{publicIPName}` | ✓ `simulators/azure/compute.go::registerPublicIPAddresses` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/publicIPAddresses` | ✓ `simulators/azure/compute.go::registerPublicIPAddresses` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | n/a | |
| `DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/publicIPAddresses/{publicIPName}` | ✓ `simulators/azure/compute.go::registerPublicIPAddresses` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | n/a | |
| `PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkInterfaces/{networkInterfaceName}` | ✓ `simulators/azure/compute.go::registerNetworkInterfaces` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkInterfaces/{networkInterfaceName}` | ✓ `simulators/azure/compute.go::registerNetworkInterfaces` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkInterfaces` | ✓ `simulators/azure/compute.go::registerNetworkInterfaces` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/networkInterfaces/{networkInterfaceName}` | ✓ `simulators/azure/compute.go::registerNetworkInterfaces` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | Supports `$expand=instanceView`. |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/instanceView` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/start` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/powerOff` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/restart` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/deallocate` | ✓ `simulators/azure/compute.go::registerVirtualMachines` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/locations/{location}/vmSizes` | ✓ `simulators/azure/compute.go::registerComputeCatalog` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | VM size discovery used around VM provisioning. |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/skus` | ✓ `simulators/azure/compute.go::registerComputeCatalog` | ✓ `simulators/azure/sdk-tests/compute_test.go` | ✓ `simulators/azure/cli-tests/compute_test.go` | ✓ `simulators/azure/terraform-tests/main.tf` | Compute SKU discovery used around VM provisioning. |

## Open subtasks staged forward

- Managed disks as standalone `Microsoft.Compute/disks` resources remain under the broader storage/disk parity audit if a future client path requires explicit disk CRUD outside VM `storageProfile.osDisk`.
