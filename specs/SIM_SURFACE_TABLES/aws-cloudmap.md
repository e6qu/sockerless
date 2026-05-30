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
| `Action Route53AutoNaming_v20170314.CreatePrivateDnsNamespace` | ✓ `simulators/aws/cloudmap.go:95::handleCMCreatePrivateDnsNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetNamespace` | ✓ `simulators/aws/cloudmap.go:96::handleCMGetNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteNamespace` | ✓ `simulators/aws/cloudmap.go:97::handleCMDeleteNamespace` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.CreateService` | ✓ `simulators/aws/cloudmap.go:98::handleCMCreateService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetService` | ✓ `simulators/aws/cloudmap.go:99::handleCMGetService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.RegisterInstance` | ✓ `simulators/aws/cloudmap.go:100::handleCMRegisterInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeregisterInstance` | ✓ `simulators/aws/cloudmap.go:101::handleCMDeregisterInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListInstances` | ✓ `simulators/aws/cloudmap.go:102::handleCMListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DiscoverInstances` | ✓ `simulators/aws/cloudmap.go:103::handleCMDiscoverInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.GetOperation` | ✓ `simulators/aws/cloudmap.go:104::handleCMGetOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListNamespaces` | ✓ `simulators/aws/cloudmap.go:105::handleCMListNamespaces` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListServices` | ✓ `simulators/aws/cloudmap.go:106::handleCMListServices` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteService` | ✓ `simulators/aws/cloudmap.go:107::handleCMDeleteService` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.ListTagsForResource` | ✓ `simulators/aws/cloudmap.go:108::handleCMListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Route53AutoNaming_v20170314.TagResource` | ✓ `simulators/aws/cloudmap.go:109::handleCMTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
