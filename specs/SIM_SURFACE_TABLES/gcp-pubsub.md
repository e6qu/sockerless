# Sim surface — gcp-pubsub

Surface registered in `simulators/gcp/pubsub.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `PUT /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:104::handlePSCreateTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:105::handlePSGetTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics` | ✓ `simulators/gcp/pubsub.go:106::handlePSListTopics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:107::handlePSDeleteTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/topics/{topicVerb}` | ✓ `simulators/gcp/pubsub.go:108::handlePSTopicVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:111::handlePSCreateSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:112::handlePSPatchSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:113::handlePSGetSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/subscriptions` | ✓ `simulators/gcp/pubsub.go:114::handlePSListSubscriptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:115::handlePSDeleteSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/subscriptions/{subVerb}` | ✓ `simulators/gcp/pubsub.go:116::handlePSSubscriptionVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:122::handlePSPatchTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:129::handlePSCreateSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:130::handlePSGetSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/snapshots` | ✓ `simulators/gcp/pubsub.go:131::handlePSListSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:132::handlePSDeleteSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
