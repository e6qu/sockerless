# Sim surface — aws-cloudmap

Surface registered in `simulators/aws/cloudmap.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Route53AutoNaming_v20170314.CreatePrivateDnsNamespace` | ✓ `simulators/aws/cloudmap.go:298::handleCMCreatePrivateDnsNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.CreatePublicDnsNamespace` | ✓ `simulators/aws/cloudmap.go:299::handleCMCreatePublicDnsNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.CreateHttpNamespace` | ✓ `simulators/aws/cloudmap.go:300::handleCMCreateHttpNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetNamespace` | ✓ `simulators/aws/cloudmap.go:301::handleCMGetNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteNamespace` | ✓ `simulators/aws/cloudmap.go:302::handleCMDeleteNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdateHttpNamespace` | ✓ `simulators/aws/cloudmap.go:303::handleCMUpdateHttpNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdatePrivateDnsNamespace` | ✓ `simulators/aws/cloudmap.go:304::handleCMUpdatePrivateDnsNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdatePublicDnsNamespace` | ✓ `simulators/aws/cloudmap.go:305::handleCMUpdatePublicDnsNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.CreateService` | ✓ `simulators/aws/cloudmap.go:306::handleCMCreateService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetService` | ✓ `simulators/aws/cloudmap.go:307::handleCMGetService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdateService` | ✓ `simulators/aws/cloudmap.go:308::handleCMUpdateService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetServiceAttributes` | ✓ `simulators/aws/cloudmap.go:309::handleCMGetServiceAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdateServiceAttributes` | ✓ `simulators/aws/cloudmap.go:310::handleCMUpdateServiceAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteServiceAttributes` | ✓ `simulators/aws/cloudmap.go:311::handleCMDeleteServiceAttributes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.RegisterInstance` | ✓ `simulators/aws/cloudmap.go:312::handleCMRegisterInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeregisterInstance` | ✓ `simulators/aws/cloudmap.go:313::handleCMDeregisterInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetInstance` | ✓ `simulators/aws/cloudmap.go:314::handleCMGetInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListInstances` | ✓ `simulators/aws/cloudmap.go:315::handleCMListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UpdateInstanceCustomHealthStatus` | ✓ `simulators/aws/cloudmap.go:316::handleCMUpdateInstanceCustomHealthStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetInstancesHealthStatus` | ✓ `simulators/aws/cloudmap.go:317::handleCMGetInstancesHealthStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DiscoverInstances` | ✓ `simulators/aws/cloudmap.go:318::handleCMDiscoverInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DiscoverInstancesRevision` | ✓ `simulators/aws/cloudmap.go:319::handleCMDiscoverInstancesRevision` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetOperation` | ✓ `simulators/aws/cloudmap.go:320::handleCMGetOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListOperations` | ✓ `simulators/aws/cloudmap.go:321::handleCMListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListNamespaces` | ✓ `simulators/aws/cloudmap.go:322::handleCMListNamespaces` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListServices` | ✓ `simulators/aws/cloudmap.go:323::handleCMListServices` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteService` | ✓ `simulators/aws/cloudmap.go:324::handleCMDeleteService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListTagsForResource` | ✓ `simulators/aws/cloudmap.go:325::handleCMListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.TagResource` | ✓ `simulators/aws/cloudmap.go:326::handleCMTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.UntagResource` | ✓ `simulators/aws/cloudmap.go:327::handleCMUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
