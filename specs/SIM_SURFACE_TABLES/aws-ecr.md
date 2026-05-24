# Sim surface — aws-ecr

Surface registered in `simulators/aws/ecr.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AmazonEC2ContainerRegistry_V20150921.CreateRepository` | ✓ `simulators/aws/ecr.go:96::handleECRCreateRepository` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribeRepositories` | ✓ `simulators/aws/ecr.go:97::handleECRDescribeRepositories` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteRepository` | ✓ `simulators/aws/ecr.go:98::handleECRDeleteRepository` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken` | ✓ `simulators/aws/ecr.go:99::handleECRGetAuthorizationToken` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchGetImage` | ✓ `simulators/aws/ecr.go:100::handleECRBatchGetImage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutImage` | ✓ `simulators/aws/ecr.go:101::handleECRPutImage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchDeleteImage` | ✓ `simulators/aws/ecr.go:102::handleECRBatchDeleteImage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.BatchCheckLayerAvailability` | ✓ `simulators/aws/ecr.go:103::handleECRBatchCheckLayerAvailability` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.PutLifecyclePolicy` | ✓ `simulators/aws/ecr.go:104::handleECRPutLifecyclePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.GetLifecyclePolicy` | ✓ `simulators/aws/ecr.go:105::handleECRGetLifecyclePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeleteLifecyclePolicy` | ✓ `simulators/aws/ecr.go:106::handleECRDeleteLifecyclePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.ListTagsForResource` | ✓ `simulators/aws/ecr.go:107::handleECRListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.TagResource` | ✓ `simulators/aws/ecr.go:108::handleECRTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.CreatePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:116::handleECRCreatePullThroughCacheRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DescribePullThroughCacheRules` | ✓ `simulators/aws/ecr.go:117::handleECRDescribePullThroughCacheRules` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AmazonEC2ContainerRegistry_V20150921.DeletePullThroughCacheRule` | ✓ `simulators/aws/ecr.go:118::handleECRDeletePullThroughCacheRule` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
