# Sim surface — azure-acr

Surface registered in `simulators/azure/acr.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /subscriptions/{subscriptionId}/providers/Microsoft.ContainerRegistry/checknameavailability` | ✓ `simulators/azure/acr.go:103::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/{path...}` | ✓ `simulators/azure/acr.go:325::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `HEAD /v2/{path...}` | ✓ `simulators/azure/acr.go:425::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/{path...}` | ✓ `simulators/azure/acr.go:457::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/{path...}` | ✓ `simulators/azure/acr.go:540::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/{path...}` | ✓ `simulators/azure/acr.go:565::func` | ✓ (direct; see coverage matrix) | n/a | n/a | manifest delete; removes digest aliases |
| `GET /acr/v1/_catalog` | ✓ `simulators/azure/acr.go:589::func` | ✓ (direct; see coverage matrix) | n/a | n/a | lists distinct repository names |
| `GET /acr/v1/{path...}` | ✓ `simulators/azure/acr.go:610::func` | ✓ (direct; see coverage matrix) | n/a | n/a | handles `{name}/_tags` and similar ACR data-plane paths |
| `PATCH /v2/{path...}` | ✓ `simulators/azure/acr.go:645::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
PR #388 (BUG-1320/1321/1322) added manifest delete (`DELETE /v2/{path...}`), catalog listing (`GET /acr/v1/_catalog`), and tag listing (`GET /acr/v1/{name}/_tags`) to support the `azcontainerregistry` data-plane SDK (`UploadManifest` → `GetManifest` → `NewListRepositoriesPager` → `NewListTagsPager` → `DeleteManifest`). SDK coverage: `simulators/azure/sdk-tests/acr_test.go` (`TestACR_ImageManifestPushGetDelete`). CLI coverage: `simulators/azure/cli-tests/acr_test.go` (`TestACR_ImageCatalogAndTags`).
<!-- HAND-WRITTEN END -->
