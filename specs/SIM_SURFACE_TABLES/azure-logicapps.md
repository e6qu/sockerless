# Sim surface — azure-logicapps

Surface registered in `simulators/azure/logicapps.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `PUT /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}` | ✓ `handleLogicWorkflowPut` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}` | ✓ `handleLogicWorkflowPatch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}` | ✓ `handleLogicWorkflowGet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}` | ✓ `handleLogicWorkflowDelete` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows` | ✓ `handleLogicWorkflowListByRG` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.Logic/workflows` | ✓ `handleLogicWorkflowListBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/disable` | ✓ `handleLogicWorkflowDisable` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/enable` | ✓ `handleLogicWorkflowEnable` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/validate` | ✓ `handleLogicWorkflowValidate` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/triggers/{triggerName}/run` | ✓ `handleLogicWorkflowTriggerRun` | ✓ (direct; see coverage matrix) | n/a | n/a | Creates a persisted workflow run record. |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/runs` | ✓ `handleLogicWorkflowRunList` | ✓ (direct; see coverage matrix) | n/a | ✓ | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/runs/{runName}` | ✓ `handleLogicWorkflowRunGet` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `POST /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Logic/workflows/{workflowName}/runs/{runName}/cancel` | ✓ `handleLogicWorkflowRunCancel` | ✓ (direct; see coverage matrix) | n/a | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
