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
| `PUT /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:137::handlePSCreateTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:138::handlePSGetTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics` | ✓ `simulators/gcp/pubsub.go:139::handlePSListTopics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:140::handlePSDeleteTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/topics/{topicVerb}` | ✓ `simulators/gcp/pubsub.go:141::handlePSTopicVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:144::handlePSCreateSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:145::handlePSPatchSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:146::handlePSGetSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/subscriptions` | ✓ `simulators/gcp/pubsub.go:147::handlePSListSubscriptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/subscriptions/{sub}` | ✓ `simulators/gcp/pubsub.go:148::handlePSDeleteSubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/subscriptions/{subVerb}` | ✓ `simulators/gcp/pubsub.go:149::handlePSSubscriptionVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/topics/{topic}` | ✓ `simulators/gcp/pubsub.go:155::handlePSPatchTopic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics/{topic}/snapshots` | ✓ `simulators/gcp/pubsub.go:161::handlePSListTopicSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/topics/{topic}/subscriptions` | ✓ `simulators/gcp/pubsub.go:162::handlePSListTopicSubscriptions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:169::handlePSCreateSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:170::handlePSPatchSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:171::handlePSGetSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/snapshots` | ✓ `simulators/gcp/pubsub.go:172::handlePSListSnapshots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/snapshots/{snap}` | ✓ `simulators/gcp/pubsub.go:173::handlePSDeleteSnapshot` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/snapshots/{snapVerb}` | ✓ `simulators/gcp/pubsub.go:174::handlePSSnapshotVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/schemas` | ✓ `simulators/gcp/pubsub.go:181::handlePSCreateSchema` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/schemas:validate` | ✓ `simulators/gcp/pubsub.go:182::handlePSValidateSchema` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/schemas:validateMessage` | ✓ `simulators/gcp/pubsub.go:183::handlePSValidateMessage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/schemas` | ✓ `simulators/gcp/pubsub.go:184::handlePSListSchemas` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/schemas/{schemaVerb}` | ✓ `simulators/gcp/pubsub.go:185::handlePSGetSchemaOrVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/schemas/{schemaVerb}` | ✓ `simulators/gcp/pubsub.go:186::handlePSSchemaPostVerb` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/schemas/{schemaVerb}` | ✓ `simulators/gcp/pubsub.go:187::handlePSDeleteSchemaOrRevision` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
