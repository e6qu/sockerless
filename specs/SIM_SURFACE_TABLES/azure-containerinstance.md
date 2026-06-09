# Sim surface — azure-containerinstance

Surface registered in `simulators/azure/containerinstance.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}` | ✓ `handleACIContainerGroupPut` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Starts real local containers in Docker-runtime mode. |
| `PATCH /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}` | ✓ `handleACIContainerGroupPatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}` | ✓ `handleACIContainerGroupGet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}` | ✓ `handleACIContainerGroupDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Stops/removes real local containers. |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups` | ✓ `handleACIContainerGroupListByRG` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.ContainerInstance/containerGroups` | ✓ `handleACIContainerGroupListBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/stop` | ✓ `handleACIContainerGroupStop` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/start` | ✓ `handleACIContainerGroupStart` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/restart` | ✓ `handleACIContainerGroupRestart` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/containers/{containerName}/logs` | ✓ `handleACIContainerLogs` | ✓ (direct; see coverage matrix) | n/a | n/a | Reads captured output from the real local container. |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/containers/{containerName}/exec` | ✓ `handleACIContainerExec` | ✓ (direct; see coverage matrix) | n/a | n/a | Returns SDK-shaped websocket session credentials. |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}/containers/{containerName}/execSessions/{sessionID}` | ✓ `handleACIContainerExecSession` | ✓ (direct; see coverage matrix) | n/a | n/a | Bridges websocket to Docker exec on the real container. |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
