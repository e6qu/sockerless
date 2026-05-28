# Sim surface — gcp-eventarc

Surface registered in `simulators/gcp/eventarc.go`. Rows below are the Eventarc v1 REST operations implemented by the GCP simulator.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | cli-test | tf-test | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/locations/{location}/triggers` | ✓ `eventarc.go::handleEventarcCreateTrigger` | ✓ `sdk-tests/eventarc_test.go::TestEventarc_TriggerLifecycleSDK` | ✓ `cli-tests/eventarc_test.go::TestEventarcCLI_TriggerLifecycle` | ✓ `google_eventarc_trigger` | Creates trigger and returns a regional long-running operation. |
| `GET /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `eventarc.go::handleEventarcGetTrigger` | ✓ | ✓ `gcloud eventarc triggers describe` | ✓ Terraform read | Returns stored trigger fields. |
| `GET /v1/projects/{project}/locations/{location}/triggers` | ✓ `eventarc.go::handleEventarcListTriggers` | ✓ iterator | ✓ `gcloud eventarc triggers list` | n/a | Lists triggers by project/location. |
| `PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `eventarc.go::handleEventarcPatchTrigger` | n/a | n/a | ✓ Terraform update path | Updates labels, filters, destination, transport, service account, and content type. |
| `DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}` | ✓ `eventarc.go::handleEventarcDeleteTrigger` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes trigger and returns a long-running operation. |
| `POST /v1/projects/{project}/locations/{location}/channels` | ✓ `eventarc.go::handleEventarcCreateChannel` | ✓ `sdk-tests/eventarc_test.go::TestEventarc_ChannelProviderConnectionSDK` | ✓ `cli-tests/eventarc_test.go::TestEventarcCLI_ChannelProviderConnection` | ✓ `google_eventarc_channel` | Creates channels and returns an AIP-style long-running operation. |
| `GET /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `eventarc.go::handleEventarcGetChannel` | ✓ | ✓ `gcloud eventarc channels describe` | ✓ Terraform read | Returns channel metadata and ACTIVE state. |
| `GET /v1/projects/{project}/locations/{location}/channels` | ✓ `eventarc.go::handleEventarcListChannels` | ✓ iterator | n/a | n/a | Lists channels by project/location. |
| `PATCH /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `eventarc.go::handleEventarcPatchChannel` | n/a | n/a | ✓ Terraform update path | Updates labels, provider, transport topic, and CMEK fields. |
| `DELETE /v1/projects/{project}/locations/{location}/channels/{channel}` | ✓ `eventarc.go::handleEventarcDeleteChannel` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes channel and returns a long-running operation. |
| `GET /v1/projects/{project}/locations/{location}/providers` | ✓ `eventarc.go::handleEventarcListProviders` | ✓ iterator | ✓ `gcloud eventarc providers list` | n/a | Lists supported Eventarc providers with event types and filtering attributes. |
| `GET /v1/projects/{project}/locations/{location}/providers/{provider}` | ✓ `eventarc.go::handleEventarcGetProvider` | ✓ | n/a | n/a | Returns provider metadata. |
| `POST /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `eventarc.go::handleEventarcCreateChannelConnection` | ✓ `TestEventarc_ChannelProviderConnectionSDK` | ✓ `gcloud eventarc channel-connections create` | n/a no provider resource | Creates channel connections and returns a long-running operation. |
| `GET /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `eventarc.go::handleEventarcGetChannelConnection` | ✓ | n/a | n/a | Returns channel connection metadata. |
| `GET /v1/projects/{project}/locations/{location}/channelConnections` | ✓ `eventarc.go::handleEventarcListChannelConnections` | ✓ iterator | ✓ `gcloud eventarc channel-connections list` | n/a | Lists channel connections by project/location. |
| `DELETE /v1/projects/{project}/locations/{location}/channelConnections/{connection}` | ✓ `eventarc.go::handleEventarcDeleteChannelConnection` | ✓ cleanup | ✓ cleanup | n/a | Deletes channel connections and returns a long-running operation. |

## Closed bugs

- BUG-1198 — foundational GCP Eventarc slice added with SDK, CLI, and Terraform coverage.
- BUG-1215 / issue #251 — channels, provider discovery/listing, and channel connections added. Terraform provider schema inspection confirmed `google_eventarc_channel` exists and no channel-connection resource exists in the current `hashicorp/google` or `hashicorp/google-beta` providers.

## Open subtasks staged forward

- No Eventarc subtasks remain from issue #251.
