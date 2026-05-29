# Sim surface — azure-compute

Surface registered in `simulators/azure/compute.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/locations/{location}/vmSizes` | ✓ `simulators/azure/compute.go:177::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.Compute/skus` | ✓ `simulators/azure/compute.go:200::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #266 closed the Azure VM lifecycle gap. `Microsoft.Network/networkInterfaces`, `Microsoft.Network/publicIPAddresses`, and `Microsoft.Compute/virtualMachines` are covered by `simulators/azure/sdk-tests/compute_test.go`, `simulators/azure/cli-tests/compute_test.go`, and `simulators/azure/terraform-tests/main.tf` through `azurerm_network_interface` and `azurerm_linux_virtual_machine`.

Issue #263 closed the Azure managed load-balancer gap for `Microsoft.Network/loadBalancers`. The simulator implements Load Balancer create/get/list/delete plus frontend IP configurations, backend address pools, probes, and load-balancing rules, including the child-resource paths used by the official clients and provider. Coverage uses `armnetwork` Load Balancer SDK coverage in `simulators/azure/sdk-tests/network_test.go`, Azure CLI `az rest` coverage in `simulators/azure/cli-tests/loadbalancer_test.go`, and Terraform `azurerm_public_ip`, `azurerm_lb`, `azurerm_lb_backend_address_pool`, `azurerm_lb_probe`, and `azurerm_lb_rule` resources in `simulators/azure/terraform-tests/main.tf`.

Issue #279 closed the Azure NAT/public-IP parity pass. `Microsoft.Network/publicIPPrefixes`, NAT Gateway list/read behavior, subnet NAT Gateway association persistence, and NAT Gateway subnet back-references are covered by `simulators/azure/sdk-tests/network_test.go`, `simulators/azure/cli-tests/nat_test.go`, and `simulators/azure/terraform-tests/main.tf` through `azurerm_public_ip_prefix`, `azurerm_nat_gateway`, `azurerm_nat_gateway_public_ip_prefix_association`, and `azurerm_subnet_nat_gateway_association`.
<!-- HAND-WRITTEN END -->
