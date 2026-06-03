# Sim surface — azure-entra

Surface registered in `simulators/azure/entra.go` and `simulators/azure/auth.go`. Covers Microsoft Entra ID (Azure AD) identity: group and user provisioning via Microsoft Graph, group membership, delegated read (`/me/memberOf`), and token grants including ROPC. The OIDC discovery and token endpoints are registered via `AzureAuthMiddleware` in `auth.go`; Graph routes are registered in `registerEntra`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

### Microsoft Graph — group provisioning

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1.0/groups` | ✓ `simulators/azure/entra.go:157::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `GET /v1.0/groups/{groupId}` | ✓ `simulators/azure/entra.go:185::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `DELETE /v1.0/groups/{groupId}` | ✓ `simulators/azure/entra.go:195::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `POST /v1.0/groups/{groupId}/members/$ref` | ✓ `simulators/azure/entra.go:205::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | user ID extracted from `@odata.id` final segment |
| `GET /v1.0/groups/{groupId}/members` | ✓ `simulators/azure/entra.go:226::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `DELETE /v1.0/groups/{groupId}/members/{userId}/$ref` | ✓ `simulators/azure/entra.go:251::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |

### Microsoft Graph — user provisioning

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1.0/users` | ✓ `simulators/azure/entra.go:263::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | creates `EntraUser` in internal store |
| `GET /v1.0/users/{userId}` | ✓ `simulators/azure/entra.go:292::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `DELETE /v1.0/users/{userId}` | ✓ `simulators/azure/entra.go:302::func` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |

### Microsoft Graph — delegated read

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /v1.0/me/memberOf` | ✓ `simulators/azure/entra.go:317::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a | n/a | returns groups from both membership store and sim-seed path |
| `GET /v1.0/me/transitiveMemberOf` | ✓ `simulators/azure/entra.go:318::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a | n/a | |

### Token grants (registered via AzureAuthMiddleware in auth.go)

| Grant type | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|
| `grant_type=authorization_code` (PKCE) | ✓ `simulators/azure/auth.go::handleAzureAuthorizationCodeToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | |
| `grant_type=client_credentials` | ✓ `simulators/azure/auth.go::handleAzureToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | |
| `grant_type=refresh_token` | ✓ `simulators/azure/auth.go::handleAzureRefreshToken` | ✓ (direct; see coverage matrix) | n/a | |
| `grant_type=password` (ROPC) | ✓ `simulators/azure/auth.go::handleAzureROPC` | ✓ (direct; see coverage matrix) | n/a | looks up user by `userPrincipalName`; 400 for unknown user |

### Sim-internal seed (non-standard; no real Azure equivalent)

| Op (verb + path) | sim handler | notes |
|---|---|---|
| `PUT /sim/v1/entra/users/{oid}` | ✓ `simulators/azure/entra.go:121::func` | backward-compat seed path; standard provisioning via Graph is preferred |
| `GET /sim/v1/entra/users/{oid}` | ✓ `simulators/azure/entra.go:139::func` | |
| `DELETE /sim/v1/entra/users/{oid}` | ✓ `simulators/azure/entra.go:144::func` | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`.
- Terraform (`azuread` provider) is blocked by BUG-1345: `hashicorp/terraform-provider-azuread` has no supported way to redirect Graph API calls to a custom endpoint. Upstream feature request: https://github.com/hashicorp/terraform-provider-azuread/issues/1837.

<!-- HAND-WRITTEN BEGIN -->
PRs #389 and #393 built the Entra identity surface. PR #389 added the sim-seed path (`/sim/v1/entra/users`) and `GET /v1.0/me/memberOf`. PR #393 replaced the seed path with standard Microsoft Graph provisioning (`POST /v1.0/groups`, `POST /v1.0/users`, `POST /v1.0/groups/{id}/members/$ref` and their GET/DELETE counterparts) and added ROPC (`grant_type=password`). SDK tests: `simulators/azure/sdk-tests/entra_test.go`. CLI tests: `simulators/azure/cli-tests/entra_test.go`.
<!-- HAND-WRITTEN END -->
