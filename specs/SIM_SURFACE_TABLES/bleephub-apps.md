# bleephub — GitHub Apps & OAuth

Surface: `bleephub/gh_apps_rest.go`, `bleephub/gh_apps_perms.go`, `bleephub/gh_apps_user_tokens.go`, `bleephub/gh_apps_oauth_mgmt.go`, `bleephub/gh_app_hooks_rest.go`, `bleephub/gh_oauth.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/apps>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## GitHub App identity

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Manifest conversion | `POST /api/v3/app-manifests/{code}/conversions` | ✓ `gh_apps_rest.go::handleManifestConversion` | ✓ `gh_apps_test.go` | |
| Get authenticated app | `GET /api/v3/app` | ✓ `handleGetAuthenticatedApp` | ✓ same | |
| Get app by slug | `GET /api/v3/apps/{app_slug}` | ✓ `handleGetAppBySlug` | ✓ same | |

## Installations

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List installations | `GET /api/v3/app/installations` | ✓ `handleListAppInstallations` | ✓ `gh_apps_test.go` | |
| Get installation | `GET /api/v3/app/installations/{id}` | ✓ `handleGetAppInstallation` | ✓ same | |
| Create installation token | `POST /api/v3/app/installations/{id}/access_tokens` | ✓ `handleCreateInstallationToken` | ✓ same | |
| Delete installation | `DELETE /api/v3/app/installations/{id}` | ✓ `handleDeleteAppInstallation` | ✓ same | |
| Suspend installation | `PUT /api/v3/app/installations/{id}/suspended` | ✓ `handleSuspendInstallation` | ✓ same | |
| Unsuspend installation | `DELETE /api/v3/app/installations/{id}/suspended` | ✓ `handleUnsuspendInstallation` | ✓ same | |
| Get repo installation | `GET /api/v3/repos/{owner}/{repo}/installation` | ✓ `handleGetRepoInstallation` | ✓ same | |
| Get org installation | `GET /api/v3/orgs/{org}/installation` | ✓ `handleGetOrgInstallation` | ✓ same | |
| Get user installation | `GET /api/v3/users/{username}/installation` | ✓ `handleGetUserInstallation` | ✓ same | |

## User installation repos

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List user installations | `GET /api/v3/user/installations` | ✓ `handleListUserInstallations` | ✓ `gh_user_installations_test.go` | |
| List user installation repos | `GET /api/v3/user/installations/{id}/repositories` | ✓ `handleListUserInstallationRepos` | ✓ same | |
| Add user installation repo | `PUT /api/v3/user/installations/{id}/repositories/{repo_id}` | ✓ `handleAddUserInstallationRepo` | ✓ same | |
| Remove user installation repo | `DELETE /api/v3/user/installations/{id}/repositories/{repo_id}` | ✓ `handleRemoveUserInstallationRepo` | ✓ same | |
| List installation repos | `GET /api/v3/installation/repositories` | ✓ `handleListInstallationRepositories` | ✓ same | |
| Revoke installation token | `DELETE /api/v3/installation/token` | ✓ `handleRevokeInstallationToken` | ✓ same | |

## App hooks

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Get hook config | `GET /api/v3/app/hook/config` | ✓ `gh_app_hooks_rest.go::handleGetAppHookConfig` | ✓ `gh_app_hooks_test.go` | |
| Update hook config | `PATCH /api/v3/app/hook/config` | ✓ `handleUpdateAppHookConfig` | ✓ same | |
| List hook deliveries | `GET /api/v3/app/hook/deliveries` | ✓ `handleListAppHookDeliveries` | ✓ same | |
| Get hook delivery | `GET /api/v3/app/hook/deliveries/{delivery_id}` | ✓ `handleGetAppHookDelivery` | ✓ same | |
| Redeliver hook | `POST /api/v3/app/hook/deliveries/{delivery_id}/attempts` | ✓ `handleRedeliverAppHookDelivery` | ✓ same | |

## OAuth app token management

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Check OAuth token | `POST /api/v3/applications/{client_id}/token` | ✓ `gh_apps_oauth_mgmt.go::handleCheckOAuthToken` | ✓ `gh_apps_oauth_mgmt_test.go` | |
| Reset OAuth token | `PATCH /api/v3/applications/{client_id}/token` | ✓ `handleResetOAuthToken` | ✓ same | |
| Revoke OAuth token | `DELETE /api/v3/applications/{client_id}/token` | ✓ `handleRevokeOAuthToken` | ✓ same | |
| Scope OAuth token | `POST /api/v3/applications/{client_id}/token/scoped` | ✓ `handleScopeOAuthToken` | ✓ same | |
| Revoke OAuth grant | `DELETE /api/v3/applications/{client_id}/grant` | ✓ `handleRevokeOAuthGrant` | ✓ same | |

## Management API (bleephub-specific)

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create app | `POST /internal/apps` | ✓ `handleCreateApp` | ✓ `gh_apps_test.go` | Internal provisioning endpoint. |
| Create installation | `POST /internal/apps/{app_id}/installations` | ✓ `handleCreateInstallationMgmt` | ✓ same | |
| Suspend installation | `POST /internal/installations/{id}/suspend` | ✓ `handleSuspendInstallationMgmt` | ✓ same | |
| Unsuspend installation | `POST /internal/installations/{id}/unsuspend` | ✓ `handleUnsuspendInstallationMgmt` | ✓ same | |
| Delete installation | `DELETE /internal/installations/{id}` | ✓ `handleDeleteInstallationMgmt` | ✓ same | |
| Create OAuth app | `POST /internal/oauth-apps` | ✓ `handleCreateOAuthAppMgmt` | ✓ `gh_apps_oauth_mgmt_test.go` | |
| List OAuth apps | `GET /internal/oauth-apps` | ✓ `handleListOAuthAppsMgmt` | ✓ same | |

## OAuth / device flows

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Device code request | `POST /login/device/code` | ✓ `gh_oauth.go::handleDeviceCode` | ✓ `gh_oauth_test.go` | |
| OAuth access token | `POST /login/oauth/access_token` | ✓ `handleOAuthAccessToken` | ✓ same | |
| Device page | `GET /login/device` | ✓ `handleDevicePage` | ✓ same | Browser-facing; renders device-activation form. |
| Login page | `GET /login` | ✓ `handleLoginPage` | ✓ same | |
| Login submit | `POST /login` | ✓ `handleLoginPost` | ✓ same | Creates session cookie for OAuth authorize flow. |
| OAuth authorize (form) | `GET /login/oauth/authorize` | ✓ `handleOAuthAuthorize` | ✓ same | Requires `_gh_sess` cookie; renders CSRF-protected form. |
| OAuth authorize (submit) | `POST /login/oauth/authorize` | ✓ `handleOAuthAuthorizeApprove` | ✓ same | Validates session + CSRF token; issues code. |
