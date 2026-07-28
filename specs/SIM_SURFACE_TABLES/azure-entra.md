# Sim surface — azure-entra

Surface registered in `simulators/azure/entra.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1.0/groups` | ✓ `simulators/azure/entra.go:228::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/groups/{groupId}` | ✓ `simulators/azure/entra.go:256::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1.0/groups/{groupId}` | ✓ `simulators/azure/entra.go:266::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/groups/{groupId}/members/$ref` | ✓ `simulators/azure/entra.go:276::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/groups/{groupId}/members` | ✓ `simulators/azure/entra.go:297::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1.0/groups/{groupId}/members/{userId}/$ref` | ✓ `simulators/azure/entra.go:324::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/users` | ✓ `simulators/azure/entra.go:336::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/users/{userId}` | ✓ `simulators/azure/entra.go:368::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PATCH /v1.0/users/{userId}` | ✓ `simulators/azure/entra.go:380::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1.0/users/{userId}` | ✓ `simulators/azure/entra.go:408::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/me/memberOf` | ✓ `simulators/azure/entra.go:425::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/me/transitiveMemberOf` | ✓ `simulators/azure/entra.go:426::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/applications` | ✓ `simulators/azure/entra.go:432::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/applications` | ✓ `simulators/azure/entra.go:455::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go:467::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PATCH /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go:476::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go:504::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/applications/{appObjectId}/addPassword` | ✓ `simulators/azure/entra.go:515::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/applications/{appObjectId}/removePassword` | ✓ `simulators/azure/entra.go:531::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/servicePrincipals` | ✓ `simulators/azure/entra.go:558::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/servicePrincipals` | ✓ `simulators/azure/entra.go:591::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go:611::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PATCH /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go:620::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go:642::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/servicePrincipals/{spId}/addPassword` | ✓ `simulators/azure/entra.go:650::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v1.0/servicePrincipals/{spId}/removePassword` | ✓ `simulators/azure/entra.go:668::func` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
The Entra surface is entirely standard Microsoft Graph: user/group/application/service-principal provisioning, membership, credential minting, and `/me` reads resolved from the bearer token's oid claim. There are no sockerless-invented seed routes and no process-global "active user": grants that carry no `login_hint` mint tokens for the directory's fixed built-in identity, and tests bind grants to specific users via `login_hint` (authorization code) or `username` (ROPC). App-registration client secrets minted via `POST /v1.0/applications/{appObjectId}/addPassword` are validated by the v2.0 `client_credentials` grant (SHA-256 stored verifier), which issues app-only tokens for the application's service principal. SDK tests: `simulators/azure/sdk-tests/entra_test.go`. CLI tests: `simulators/azure/cli-tests/entra_test.go`.
<!-- HAND-WRITTEN END -->
