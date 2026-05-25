# Sim surface — aws-cloudmap

Surface registered in `simulators/aws/cloudmap.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Route53AutoNaming_v20170314.CreatePrivateDnsNamespace` | ✓ `simulators/aws/cloudmap.go:95::handleCMCreatePrivateDnsNamespace` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.GetNamespace` | ✓ `simulators/aws/cloudmap.go:96::handleCMGetNamespace` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteNamespace` | ✓ `simulators/aws/cloudmap.go:97::handleCMDeleteNamespace` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.CreateService` | ✓ `simulators/aws/cloudmap.go:98::handleCMCreateService` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.GetService` | ✓ `simulators/aws/cloudmap.go:99::handleCMGetService` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.RegisterInstance` | ✓ `simulators/aws/cloudmap.go:100::handleCMRegisterInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.DeregisterInstance` | ✓ `simulators/aws/cloudmap.go:101::handleCMDeregisterInstance` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.ListInstances` | ✓ `simulators/aws/cloudmap.go:102::handleCMListInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.DiscoverInstances` | ✓ `simulators/aws/cloudmap.go:103::handleCMDiscoverInstances` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.GetOperation` | ✓ `simulators/aws/cloudmap.go:104::handleCMGetOperation` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.ListNamespaces` | ✓ `simulators/aws/cloudmap.go:105::handleCMListNamespaces` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.ListServices` | ✓ `simulators/aws/cloudmap.go:106::handleCMListServices` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.DeleteService` | ✓ `simulators/aws/cloudmap.go:107::handleCMDeleteService` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.ListTagsForResource` | ✓ `simulators/aws/cloudmap.go:108::handleCMListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Route53AutoNaming_v20170314.TagResource` | ✓ `simulators/aws/cloudmap.go:109::handleCMTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
