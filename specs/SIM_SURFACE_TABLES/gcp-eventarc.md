# Sim surface — gcp-eventarc

Surface registered in `simulators/gcp/eventarc.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:89::handleGCPRegionalTriggerCreate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:90::handleGCPRegionalTriggerList` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:91::handleGCPRegionalTriggerGet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:92::handleGCPRegionalTriggerPatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:93::handleGCPRegionalTriggerDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:94::handleEventarcCreateChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:95::handleEventarcListChannels` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:96::handleEventarcGetChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:97::handleEventarcPatchChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:98::handleEventarcDeleteChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers` | ✓ `simulators/gcp/eventarc.go:99::handleEventarcListProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers/{provider}` | ✓ `simulators/gcp/eventarc.go:100::handleEventarcGetProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:101::handleEventarcCreateChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:102::handleEventarcListChannelConnections` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:103::handleEventarcGetChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:104::handleEventarcDeleteChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
