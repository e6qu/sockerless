# Sim surface — gcp-eventarc

Surface registered in `simulators/gcp/eventarc.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:89::handleGCPRegionalTriggerCreate` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:90::handleGCPRegionalTriggerList` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:91::handleGCPRegionalTriggerGet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:92::handleGCPRegionalTriggerPatch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:93::handleGCPRegionalTriggerDelete` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:94::handleEventarcCreateChannel` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:95::handleEventarcListChannels` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:96::handleEventarcGetChannel` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:97::handleEventarcPatchChannel` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:98::handleEventarcDeleteChannel` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers` | ✓ `simulators/gcp/eventarc.go:99::handleEventarcListProviders` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers/{provider}` | ✓ `simulators/gcp/eventarc.go:100::handleEventarcGetProvider` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:101::handleEventarcCreateChannelConnection` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:102::handleEventarcListChannelConnections` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:103::handleEventarcGetChannelConnection` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:104::handleEventarcDeleteChannelConnection` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
