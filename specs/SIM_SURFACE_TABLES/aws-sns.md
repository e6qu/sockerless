# Sim surface — aws-sns

Surface registered in `simulators/aws/sns.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateTopic` | ✓ `simulators/aws/sns.go:67::handleSNSCreateTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteTopic` | ✓ `simulators/aws/sns.go:68::handleSNSDeleteTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListTopics` | ✓ `simulators/aws/sns.go:69::handleSNSListTopics` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetTopicAttributes` | ✓ `simulators/aws/sns.go:70::handleSNSGetTopicAttributes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Subscribe` | ✓ `simulators/aws/sns.go:71::handleSNSSubscribe` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Unsubscribe` | ✓ `simulators/aws/sns.go:72::handleSNSUnsubscribe` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListSubscriptions` | ✓ `simulators/aws/sns.go:73::handleSNSListSubscriptions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListSubscriptionsByTopic` | ✓ `simulators/aws/sns.go:74::handleSNSListSubscriptionsByTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action Publish` | ✓ `simulators/aws/sns.go:75::handleSNSPublish` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action TagResource` | ✓ `simulators/aws/sns.go:76::handleSNSTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action UntagResource` | ✓ `simulators/aws/sns.go:77::handleSNSUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/sns.go:78::handleSNSListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
