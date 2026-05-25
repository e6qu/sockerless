# Sim surface — gcp-pubsub

Surface registered in `simulators/gcp/pubsub.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `PUT /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:103::handlePSCreateTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:104::handlePSGetTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/topics` | ✓ `simulators/gcp/pubsub.go:105::handlePSListTopics` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:106::handlePSDeleteTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/topics/{topicVerb}` | ✓ `simulators/gcp/pubsub.go:107::handlePSTopicVerb` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:110::handlePSCreateSubscription` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:111::handlePSPatchSubscription` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:112::handlePSGetSubscription` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/subscriptions` | ✓ `simulators/gcp/pubsub.go:113::handlePSListSubscriptions` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:114::handlePSDeleteSubscription` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/subscriptions/{subVerb}` | ✓ `simulators/gcp/pubsub.go:115::handlePSSubscriptionVerb` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:121::handlePSPatchTopic` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
