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
| `POST /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:119::handleGCPRegionalTriggerCreate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers` | ✓ `simulators/gcp/eventarc.go:120::handleGCPRegionalTriggerList` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:121::handleGCPRegionalTriggerGet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:122::handleGCPRegionalTriggerPatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `simulators/gcp/eventarc.go:123::handleGCPRegionalTriggerDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/triggers/{triggerAction}` | ✓ `simulators/gcp/eventarc.go:124::handleEventarcTriggerIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:125::handleEventarcCreateChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels` | ✓ `simulators/gcp/eventarc.go:126::handleEventarcListChannels` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:127::handleEventarcGetChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:128::handleEventarcPatchChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `simulators/gcp/eventarc.go:129::handleEventarcDeleteChannel` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channels/{channelAction}` | ✓ `simulators/gcp/eventarc.go:130::handleEventarcChannelIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers` | ✓ `simulators/gcp/eventarc.go:131::handleEventarcListProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/providers/{provider}` | ✓ `simulators/gcp/eventarc.go:132::handleEventarcGetProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:133::handleEventarcCreateChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `simulators/gcp/eventarc.go:134::handleEventarcListChannelConnections` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:135::handleEventarcGetChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `simulators/gcp/eventarc.go:136::handleEventarcDeleteChannelConnection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/channelConnections/{connectionAction}` | ✓ `simulators/gcp/eventarc.go:137::handleEventarcChannelConnectionIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/enrollments` | ✓ `simulators/gcp/eventarc.go:140::handleEventarcCreateEnrollment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/enrollments` | ✓ `simulators/gcp/eventarc.go:141::handleEventarcListEnrollments` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/enrollments/{enrollment}` | ✓ `simulators/gcp/eventarc.go:142::handleEventarcGetEnrollment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/enrollments/{enrollment}` | ✓ `simulators/gcp/eventarc.go:143::handleEventarcPatchEnrollment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/enrollments/{enrollment}` | ✓ `simulators/gcp/eventarc.go:144::handleEventarcDeleteEnrollment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/enrollments/{enrollmentAction}` | ✓ `simulators/gcp/eventarc.go:145::handleEventarcEnrollmentIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/messageBuses` | ✓ `simulators/gcp/eventarc.go:148::handleEventarcCreateMessageBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/messageBuses` | ✓ `simulators/gcp/eventarc.go:149::handleEventarcListMessageBuses` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/messageBuses/{bus}` | ✓ `simulators/gcp/eventarc.go:150::handleEventarcMessageBusGet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/messageBuses/{bus}` | ✓ `simulators/gcp/eventarc.go:151::handleEventarcPatchMessageBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/messageBuses/{bus}` | ✓ `simulators/gcp/eventarc.go:152::handleEventarcDeleteMessageBus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/messageBuses/{busAction}` | ✓ `simulators/gcp/eventarc.go:153::handleEventarcMessageBusIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/pipelines` | ✓ `simulators/gcp/eventarc.go:156::handleEventarcCreatePipeline` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/pipelines` | ✓ `simulators/gcp/eventarc.go:157::handleEventarcListPipelines` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/pipelines/{pipeline}` | ✓ `simulators/gcp/eventarc.go:158::handleEventarcGetPipeline` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/pipelines/{pipeline}` | ✓ `simulators/gcp/eventarc.go:159::handleEventarcPatchPipeline` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/pipelines/{pipeline}` | ✓ `simulators/gcp/eventarc.go:160::handleEventarcDeletePipeline` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/pipelines/{pipelineAction}` | ✓ `simulators/gcp/eventarc.go:161::handleEventarcPipelineIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/googleApiSources` | ✓ `simulators/gcp/eventarc.go:164::handleEventarcCreateGoogleAPISource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/googleApiSources` | ✓ `simulators/gcp/eventarc.go:165::handleEventarcListGoogleAPISources` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/googleApiSources/{source}` | ✓ `simulators/gcp/eventarc.go:166::handleEventarcGetGoogleAPISource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/googleApiSources/{source}` | ✓ `simulators/gcp/eventarc.go:167::handleEventarcPatchGoogleAPISource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/googleApiSources/{source}` | ✓ `simulators/gcp/eventarc.go:168::handleEventarcDeleteGoogleAPISource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/googleApiSources/{sourceAction}` | ✓ `simulators/gcp/eventarc.go:169::handleEventarcGoogleAPISourceIAMAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/googleChannelConfig` | ✓ `simulators/gcp/eventarc.go:172::handleEventarcGetGoogleChannelConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v1/projects/{project}/locations/{location}/googleChannelConfig` | ✓ `simulators/gcp/eventarc.go:173::handleEventarcUpdateGoogleChannelConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations` | ✓ `simulators/gcp/eventarc.go:176::handleEventarcListLocations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}` | ✓ `simulators/gcp/eventarc.go:177::handleEventarcGetLocation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/operations` | ✓ `simulators/gcp/eventarc.go:178::handleEventarcListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/operations/{operation}` | ✓ `simulators/gcp/eventarc.go:179::handleEventarcDeleteOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
