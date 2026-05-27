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

## Closed bugs

- BUG-1198 — foundational GCP Eventarc slice added with SDK, CLI, and Terraform coverage.

## Open subtasks staged forward

- BUG-1215 / issue #251 tracks remaining Eventarc parity for channels, channel connections, provider discovery/listing, and sibling surfaces exposed by official clients.
