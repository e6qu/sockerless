# Sim surface — azure-monitor

Surface registered in `simulators/azure/insights.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `PUT /subscriptions/{s}/resourceGroups/{rg}/providers/Microsoft.Insights/components/{name}` | ✓ `simulators/azure/insights.go:40::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | upsert; preserves instrumentation key across updates |
| `GET /subscriptions/{s}/resourceGroups/{rg}/providers/Microsoft.Insights/components/{name}` | ✓ `simulators/azure/insights.go:94::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /subscriptions/{s}/resourceGroups/{rg}/providers/Microsoft.Insights/components/{name}` | ✓ `simulators/azure/insights.go:112::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{s}/resourceGroups/{rg}/providers/Microsoft.Insights/components/{name}/currentbillingfeatures` | ✓ `simulators/azure/insights.go:138::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /subscriptions/{s}/resourceGroups/{rg}/providers/Microsoft.Insights/components/{name}/currentbillingfeatures` | ✓ `simulators/azure/insights.go:141::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/apps/{appId}/query` | ✓ `simulators/azure/insights.go:143::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/workspaces/{workspaceId}/query` | ✓ `simulators/azure/monitor.go:381::queryHandler` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /workspaces/{workspaceId}/query` | ✓ `simulators/azure/monitor.go:382::queryHandler` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /dataCollectionRules/{dcrId}/streams/{streamName}` | ✓ `simulators/azure/monitor.go:385::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
PR #388 (BUG-1316/1317) added Application Insights component CRUD (`PUT/GET/DELETE .../Microsoft.Insights/components/{name}`) and billing features. The upsert handler preserves the instrumentation key across updates (real App Insights keeps the same key). SDK coverage: `simulators/azure/sdk-tests/insights_test.go`. CLI coverage: `simulators/azure/cli-tests/monitor_test.go`. Terraform coverage: `simulators/azure/terraform-tests/main.tf` (`azurerm_application_insights`).
<!-- HAND-WRITTEN END -->
