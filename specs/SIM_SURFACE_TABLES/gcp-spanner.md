# Sim surface — gcp-spanner

Surface registered in `simulators/gcp/spanner.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /spanner/v1/projects/{project}/instanceConfigOperations` | ✓ `simulators/gcp/spanner.go:106::handleSpannerListInstanceConfigOperationsCollection` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /spanner/v1/scans` | ✓ `simulators/gcp/spanner.go:107::handleSpannerListScans` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
The session data plane is mounted by the dynamic Cloud Spanner child router,
so the extractor cannot recover these custom-method paths from individual
`HandleFunc` calls. The official Google Cloud SDK exercised both REST and gRPC;
`gcloud spanner databases execute-sql` exercised SQL and partitioned DML over
REST. Terraform covered the administrative instance/database/DDL surface; the
official provider exposed no resource for session-scoped data-plane calls.

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST .../sessions:batchCreate` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerBatchCreateSessionsREST` | ✓ `spanner_rest_data_test.go` | n/a | n/a | Official REST SDK |
| `POST .../sessions/{session}:executeSql` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Real SQL query and DML execution |
| `POST .../sessions/{session}:executeStreamingSql` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Complete `PartialResultSet` stream |
| `POST .../sessions/{session}:executeBatchDml` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Ordered execution, replay, and partial status |
| `POST .../sessions/{session}:read` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Transactional key-set read |
| `POST .../sessions/{session}:streamingRead` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Complete `PartialResultSet` stream |
| `POST .../sessions/{session}:beginTransaction` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Real SQLite transaction lifecycle |
| `POST .../sessions/{session}:commit` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Commits DML and mutations atomically |
| `POST .../sessions/{session}:rollback` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Discards uncommitted changes |
| `POST .../sessions/{session}:partitionQuery` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ `spanner_rest_data_test.go` | n/a | n/a | Opaque token bound to request and read-only transaction |
| `POST .../sessions/{session}:partitionRead` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ `spanner_rest_data_test.go` | n/a | n/a | Opaque token bound to request and read-only transaction |
| `POST .../sessions/{session}:batchWrite` | ✓ `simulators/gcp/spanner_rest.go::handleSpannerSessionActionREST` | ✓ REST + gRPC | n/a | n/a | Independent atomic mutation groups |
<!-- HAND-WRITTEN END -->
