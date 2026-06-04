# bleephub — Users & misc

Surface: `bleephub/gh_rest.go`, `bleephub/gh_misc_endpoints.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/users>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## User identity

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| API root | `GET /api/v3/` | ✓ `gh_rest.go::handleGHApiRoot` | ✓ `gh_conformance_test.go` | Returns links map; requires auth. |
| Get authenticated user | `GET /api/v3/user` | ✓ `handleGHUser` | ✓ `gh_test.go` | |
| Get user by login | `GET /api/v3/users/{username}` | ✓ `handleGHUserByLogin` | ✓ `gh_test.go` | |
| Rate limit | `GET /api/v3/rate_limit` | ✓ `handleGHRateLimit` | ✓ `gh_test.go` | Returns per-resource rate buckets. |

## SSH keys

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List authenticated user keys | `GET /api/v3/user/keys` | ✓ `handleListUserKeys` | ✓ `gh_misc_endpoints_decode_test.go` | |
| Create user key | `POST /api/v3/user/keys` | ✓ `handleCreateUserKey` | ✓ same | |
| Get user key | `GET /api/v3/user/keys/{key_id}` | ✓ `handleGetUserKey` | ✓ same | |
| Delete user key | `DELETE /api/v3/user/keys/{key_id}` | ✓ `handleDeleteUserKey` | ✓ same | |
| List keys by login | `GET /api/v3/users/{username}/keys` | ✓ `handleListUserKeysByLogin` | ✓ same | |

## GPG keys

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List GPG keys | `GET /api/v3/user/gpg_keys` | ✓ `handleListGPGKeys` | ✓ `gh_misc_endpoints_decode_test.go` | |
| List GPG keys by login | `GET /api/v3/users/{username}/gpg_keys` | ✓ `handleListGPGKeysByLogin` | ✓ same | |

## Emails

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List emails | `GET /api/v3/user/emails` | ✓ `handleListUserEmails` | ✓ `gh_misc_endpoints_decode_test.go` | |

## Followers / following

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List followers | `GET /api/v3/users/{username}/followers` | ✓ `handleListFollowers` | ✓ `gh_misc_endpoints_decode_test.go` | |
| List following | `GET /api/v3/users/{username}/following` | ✓ `handleListFollowing` | ✓ same | |
| List my followers | `GET /api/v3/user/followers` | ✓ `handleListMyFollowers` | ✓ same | |
| List my following | `GET /api/v3/user/following` | ✓ `handleListMyFollowing` | ✓ same | |
| Follow user | `PUT /api/v3/user/following/{username}` | ✓ `handleFollowUser` | ✓ same | |
| Unfollow user | `DELETE /api/v3/user/following/{username}` | ✓ `handleUnfollowUser` | ✓ same | |

## OIDC / Actions identity

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Actions OIDC token | `GET /token` | ✓ `handleActionsOIDCToken` | ✓ `gh_actions_test.go` | Returns signed JWT for Actions OIDC. |
| OIDC discovery | `GET /.well-known/openid-configuration` | ✓ `handleOIDCDiscovery` | ✓ same | Advertises JWKS endpoint and supported claims. |
