# Terraform Google Provider Minimum Waits

This note records upstream wait behavior that affects the GCP simulator
Terraform suite. It is not simulator behavior. These waits come from
`hashicorp/terraform-provider-google` v7.34.0, which is the version pinned by the
GCP Terraform harness.

Terraform resource `timeouts {}` blocks cap an operation's total deadline. They
do not remove provider waiters. The Google provider is different from AWS in one
important way: it exposes a provider-level `poll_interval` argument, but the
shared waiter still enforces a hard minimum sleep and some resources add
resource-specific status waits.

## Shared Operation Waiter

Most Google resources wait on service long-running operations through the shared
operation waiter:

- [`google/tpgresource/common_operation.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/tpgresource/common_operation.go#L155-L168) sets `MinTimeout: 2s` and uses the configured `PollInterval`.
- [`google/transport/config.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/transport/config.go#L393-L394) defaults `PollInterval` to `10s` when unset.
- [`google/provider/provider.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/provider/provider.go#L158-L161) exposes `poll_interval`, and [`provider.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/provider/provider.go#L1142-L1147) parses it as a Go duration.

The simulator harness does not set `poll_interval`, so the pinned provider's
default is `10s` with a `2s` minimum waiter sleep.

## Resource-Specific Waits Relevant To Current Coverage

| Surface | Provider source | Hard wait behavior |
|---|---|---|
| Compute Engine operations | [`google/services/compute/compute_operation.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/services/compute/compute_operation.go#L139-L157) | Uses the shared operation waiter and the provider `PollInterval`. |
| Compute instance post-operation status | [`google/services/compute/resource_compute_instance.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/services/compute/resource_compute_instance.go#L1823-L1841) | Adds a resource-level status waiter with `Delay: 5s` and `MinTimeout: 2s` after operations such as create/start/stop. |
| Cloud Run v2 operations | [`google/services/cloudrunv2/cloud_run_v2_operation.go`](https://github.com/hashicorp/terraform-provider-google/blob/v7.34.0/google/services/cloudrunv2/cloud_run_v2_operation.go#L97-L107) | Uses the shared operation waiter when the API returns an operation name; synchronous responses skip this wait. |

## Test Implication

For GCP, test runtime can be affected by the provider-level `poll_interval`, but
changing it would make the harness less representative of normal provider
defaults. Keep default-provider coverage as the baseline. If a future GCP
Terraform package exceeds the 5-minute cap because of accumulated real provider
waits, split the package while preserving real provider apply/read/destroy
coverage.
