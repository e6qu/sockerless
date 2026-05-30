# Terraform Azure Provider Minimum Waits

This note records upstream wait behavior that affects the Azure simulator
Terraform suite. It is not simulator behavior. These waits come from the
providers pinned by the Azure Terraform harness:

- `hashicorp/azurerm` v4.74.0
- `hashicorp/azurestack` v1.0.0

Terraform resource `timeouts {}` blocks cap an operation's total deadline. They
do not generally tune Azure ARM long-running-operation poll cadence.

## AzureRM Provider

AzureRM wraps resource operations in timeout contexts and delegates most ARM
long-running operations to the vendored Azure SDK pollers:

- [`internal/timeouts/determine.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/internal/timeouts/determine.go#L13-L58) wraps create/read/update/delete contexts with the configured operation timeout.
- [`vendor/.../resourcemanager/poller_provisioning_state.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/vendor/github.com/hashicorp/go-azure-sdk/sdk/client/resourcemanager/poller_provisioning_state.go#L21-L67) sets the default ARM provisioning-state polling interval to `10s`.
- [`vendor/.../resourcemanager/poller_lro.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/vendor/github.com/hashicorp/go-azure-sdk/sdk/client/resourcemanager/poller_lro.go#L28-L47) uses a `10s` initial retry duration for Azure LRO polling unless the service response supplies `Retry-After`.
- [`vendor/.../pollers/poller.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/vendor/github.com/hashicorp/go-azure-sdk/sdk/client/pollers/poller.go#L120-L149) sleeps between polls, then calls the ARM poller until terminal status.

Some AzureRM resources also add explicit Terraform SDK waiters on top of ARM
polling. In current simulator coverage, Linux VM delete is the visible example:

| Surface | Provider source | Hard wait behavior |
|---|---|---|
| Linux VM delete confirmation | [`internal/services/compute/linux_virtual_machine_resource.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/internal/services/compute/linux_virtual_machine_resource.go#L1853-L1860) | After delete, if a GET still returns 200, the provider waits for 404 with `MinTimeout: 30s`. |

The vendored poller has a context flag to skip polling delay, but it is an
internal SDK mechanism rather than a public Terraform configuration knob:
[`pollers/context.go`](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.74.0/vendor/github.com/hashicorp/go-azure-sdk/sdk/client/pollers/context.go#L14-L25).

## Azure Stack Provider

The Azure Stack provider uses operation timeouts plus ARM futures:

- [`internal/tf/timeouts/determine.go`](https://github.com/hashicorp/terraform-provider-azurestack/blob/v1.0.0/internal/tf/timeouts/determine.go#L10-L56) wraps CRUD contexts with configured operation timeouts.
- `azurestack_storage_account` defaults to 60-minute create/update/delete timeouts and waits on ARM futures with `WaitForCompletionRef`: [`storage_account_resource.go`](https://github.com/hashicorp/terraform-provider-azurestack/blob/v1.0.0/internal/services/storage/storage_account_resource.go#L47-L51), [`storage_account_resource.go`](https://github.com/hashicorp/terraform-provider-azurestack/blob/v1.0.0/internal/services/storage/storage_account_resource.go#L297).
- `azurestack_virtual_network` defaults to 30-minute create/update/delete timeouts and adds a post-create waiter with `MinTimeout: 1m`: [`virtual_network_resource.go`](https://github.com/hashicorp/terraform-provider-azurestack/blob/v1.0.0/internal/services/network/virtual_network_resource.go#L37-L41), [`virtual_network_resource.go`](https://github.com/hashicorp/terraform-provider-azurestack/blob/v1.0.0/internal/services/network/virtual_network_resource.go#L182-L187).

## Test Implication

For Azure, provider runtime is dominated by ARM LRO polling and any
resource-specific Terraform SDK waiters layered on top. If a future Azure
Terraform package exceeds the repository's 5-minute cap, split the package while
keeping real `azurerm` / `azurestack` provider apply/read/destroy coverage.
Do not bypass ARM operation routes, lengthen the cap, or mock provider
completion.
