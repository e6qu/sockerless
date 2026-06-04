# bleephub — Webhooks

Surface: `bleephub/gh_hooks_rest.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/repos/webhooks>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Repository webhooks

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create hook | `POST /api/v3/repos/{owner}/{repo}/hooks` | ✓ `gh_hooks_rest.go::handleCreateHook` | ✓ `gh_hooks_test.go::TestHooks_CRUD` | Response includes `url`, `test_url`, `ping_url`, `deliveries_url`, `last_response` per GitHub schema. |
| List hooks | `GET /api/v3/repos/{owner}/{repo}/hooks` | ✓ `handleListHooks` | ✓ `TestHooks_CRUD` | |
| Get hook | `GET /api/v3/repos/{owner}/{repo}/hooks/{id}` | ✓ `handleGetHook` | ✓ `TestHooks_CRUD` | |
| Update hook | `PATCH /api/v3/repos/{owner}/{repo}/hooks/{id}` | ✓ `handleUpdateHook` | ✓ `TestHooks_CRUD` | |
| Delete hook | `DELETE /api/v3/repos/{owner}/{repo}/hooks/{id}` | ✓ `handleDeleteHook` | ✓ `TestHooks_CRUD` | Returns 204 No Content. |
| Ping hook | `POST /api/v3/repos/{owner}/{repo}/hooks/{id}/pings` | ✓ `handlePingHook` | ✓ `TestHooks_Ping` | Async; triggers delivery to hook URL. |

## Webhook deliveries

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List deliveries | `GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries` | ✓ `handleListHookDeliveries` | ✓ `TestHooks_Ping` | Delivery wire shape verified: `status`, `url`, `installation_id`, `repository_id` per GitHub schema. |
| Get delivery | `GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}` | ✓ `handleGetHookDelivery` | ✓ `TestHooks_Ping` | Full delivery includes `request` + `response` sub-objects. |
| Redeliver | `POST /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}/attempts` | ✓ `handleRedeliverHookDelivery` | ✓ `TestHooks_Deliveries_Redeliver` | New delivery with `redelivery=true`. Returns 202 + `{id, redelivery: true}`. |
