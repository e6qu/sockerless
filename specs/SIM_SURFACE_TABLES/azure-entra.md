# Sim surface — azure-entra

Surface registered in `simulators/azure/entra.go` and `simulators/azure/auth.go`. Covers Microsoft Entra ID (Azure AD) identity: group, user, application, and service-principal provisioning via Microsoft Graph, group membership, client-secret credentials (`addPassword`/`removePassword`), delegated read (`/me/memberOf`), and token grants including ROPC and secret-validated `client_credentials`. The OIDC discovery and token endpoints are registered via `AzureAuthMiddleware` in `auth.go`; Graph routes are registered in `registerEntra`.

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
| `GET /v1.0/me/memberOf` | ✓ `simulators/azure/entra.go::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a | n/a | user resolved from the bearer token's oid; 401 without a bearer |
| `GET /v1.0/me/transitiveMemberOf` | ✓ `simulators/azure/entra.go::handleGraphMemberOf` | ✓ (direct; see coverage matrix) | n/a | n/a | |

### Microsoft Graph — applications + service principals

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1.0/applications` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `GET /v1.0/applications` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `GET /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `PATCH /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `DELETE /v1.0/applications/{appObjectId}` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `POST /v1.0/applications/{appObjectId}/addPassword` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | secretText returned exactly once; the v2.0 `client_credentials` grant validates these credentials |
| `POST /v1.0/applications/{appObjectId}/removePassword` | ✓ `simulators/azure/entra.go::registerEntraApplications` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `POST /v1.0/servicePrincipals` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `GET /v1.0/servicePrincipals` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | `$filter=appId eq '…'` supported |
| `GET /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `PATCH /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `DELETE /v1.0/servicePrincipals/{spId}` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |
| `POST /v1.0/servicePrincipals/{spId}/addPassword` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | secretText returned exactly once |
| `POST /v1.0/servicePrincipals/{spId}/removePassword` | ✓ `simulators/azure/entra.go::registerEntraServicePrincipals` | ✓ (direct; see coverage matrix) | ✗ BUG-1345 | n/a | |

### Token grants (registered via AzureAuthMiddleware in auth.go)

| Grant type | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|
| `grant_type=authorization_code` (PKCE) | ✓ `simulators/azure/auth.go::handleAzureAuthorizationCodeToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | |
| `grant_type=client_credentials` | ✓ `simulators/azure/auth.go::handleAzureToken` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | a client_id registered as a Graph application validates its `client_secret` against the app registration's password credentials (`handleAzureRegisteredAppClientCredentials`) and mints an app-only token for the service principal; a client_id the directory holds no application for is rejected with `unauthorized_client` AADSTS700016 (`400`), matching real Microsoft Entra. The well-known bootstrap application (`test-client-id`/`test-client-secret`, seeded in `entra.go::init`) stands in for an administrator-provisioned app registration, the same role the AWS simulator's seeded `test`/`test` credential plays |
| `grant_type=refresh_token` | ✓ `simulators/azure/auth.go::handleAzureRefreshToken` | ✓ (direct; see coverage matrix) | n/a | |
| `grant_type=password` (ROPC) | ✓ `simulators/azure/auth.go::handleAzureROPC` | ✓ (direct; see coverage matrix) | n/a | looks up user by `userPrincipalName`; 400 for unknown user |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`.
- Terraform (`azuread` provider) is blocked by BUG-1345: `hashicorp/terraform-provider-azuread` has no supported way to redirect Graph API calls to a custom endpoint. Upstream feature request: https://github.com/hashicorp/terraform-provider-azuread/issues/1837.

<!-- HAND-WRITTEN BEGIN -->
The Entra surface is entirely standard Microsoft Graph: user/group/application/service-principal provisioning, membership, credential minting, and `/me` reads resolved from the bearer token's oid claim. There are no sockerless-invented seed routes and no process-global "active user": grants that carry no `login_hint` mint tokens for the directory's fixed built-in identity, and tests bind grants to specific users via `login_hint` (authorization code) or `username` (ROPC). App-registration client secrets minted via `POST /v1.0/applications/{appObjectId}/addPassword` are validated by the v2.0 `client_credentials` grant (SHA-256 stored verifier), which issues app-only tokens for the application's service principal. SDK tests: `simulators/azure/sdk-tests/entra_test.go`. CLI tests: `simulators/azure/cli-tests/entra_test.go`.
<!-- HAND-WRITTEN END -->
