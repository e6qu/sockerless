# Azure Key Vault — data plane

Surface: `simulators/azure/keyvault.go` (data plane). All operations dispatch via the `<vault>.vault.<sim-host>` subdomain wrapper. Every authenticated request goes through the `WWW-Authenticate: Bearer` challenge-then-retry handshake; the authorization URL emitted by the sim **must** have ≥ 4 path-split segments because the Azure SDK indexes `parts[3]` without a bounds check (issue #193 → BUG-1135 → BUG-1143).

Canonical reference: <https://learn.microsoft.com/en-us/rest/api/keyvault/>

## Status legend

- ✓ — implemented + tested
- ✗ — missing
- 501 — stubbed with `NotImplemented` envelope
- n/a — no terraform-provider resource exists

## Common (every data-plane request)

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| Challenge handshake | any `<vault>.vault.<host>/...` w/o `Authorization` | ✓ `keyvault.go::registerKeyVault` (WrapHandler) + `handleKeyVaultDataPlane` | ✓ `keyvault_sdk_test.go::TestKeyVault_SDK_Secrets_ChallengeRoundTrip` + Keys + Certificates | n/a | URL format must split to ≥ 4 segments (BUG-1143). |

## Secrets

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| SetSecret | `PUT /secrets/{name}` | ✓ `keyvault.go::handleKVSetSecret` | ✓ `TestKeyVault_SDK_Secrets_ChallengeRoundTrip` | ✗ | tf: `azurerm_key_vault_secret` |
| GetSecret | `GET /secrets/{name}` | ✓ `handleKVGetSecret` | ✓ same | ✗ | |
| GetSecret (specific version) | `GET /secrets/{name}/{version}` | ✓ same | ✓ `TestKeyVault_State_FullVersionChain` | ✗ | |
| ListSecrets | `GET /secrets` | ✓ `handleKVListSecrets` | ✗ | ✗ | |
| ListSecretVersions | `GET /secrets/{name}/versions` | ✓ same | ✓ `TestKeyVault_State_FullVersionChain` | ✗ | SDK pager. |
| DeleteSecret | `DELETE /secrets/{name}` | ✓ `handleKVDeleteSecret` | ✓ `TestKeyVault_SDK_Secrets_ChallengeRoundTrip` + `TestKeyVault_State_SoftDeleteRoundTrip` | ✗ | |
| UpdateSecret | `PATCH /secrets/{name}/{version}` | ✓ `handleKVPatchSecret` | ✗ | ✗ | Used by `azurerm_key_vault_secret` updates. |
| BackupSecret / RestoreSecret | `POST /secrets/{name}/backup` / `/secrets/restore` | ✗ | ✗ | ✗ | Low-frequency; surface 501 if a runner scenario hits them. |
| RecoverDeletedSecret / PurgeDeletedSecret | `POST /deletedsecrets/{name}/recover` / `DELETE /deletedsecrets/{name}` | ✓ `handleKVRecoverDeletedSecret` / `handleKVPurgeDeletedSecret` | ✓ `TestKeyVault_State_SoftDeleteRoundTrip` | ✗ | Soft-delete state machine. |

## Keys

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| CreateKey | `POST /keys/{name}/create` | ✓ `keyvault.go::handleKVCreateKey` | ✓ `TestKeyVault_SDK_Keys_ChallengeRoundTrip` | ✗ | tf: `azurerm_key_vault_key` |
| GetKey | `GET /keys/{name}` | ✓ `handleKVGetKey` | ✓ same | ✗ | |
| GetKey (version) | `GET /keys/{name}/{version}` | ✓ same | ✗ | ✗ | |
| ListKeys | `GET /keys` | ✓ `handleKVListKeys` | ✗ | ✗ | |
| ListKeyVersions | `GET /keys/{name}/versions` | ✓ same | ✗ | ✗ | |
| DeleteKey | `DELETE /keys/{name}` | ✓ `handleKVDeleteKey` | ✗ | ✗ | |
| UpdateKey | `PATCH /keys/{name}/{version}` | ✗ | ✗ | ✗ | |
| ImportKey | `PUT /keys/{name}` | ✗ | ✗ | ✗ | |
| Sign / Verify / Encrypt / Decrypt / WrapKey / UnwrapKey | `POST /keys/{name}/{version}/{op}` | ✗ | ✗ | ✗ | Crypto operations; sim doesn't model real key material. |
| BackupKey / RestoreKey | `POST /keys/{name}/backup` / `/keys/restore` | ✗ | ✗ | ✗ | |

## Certificates

| Operation | Verb + path | sim handler | sdk-test | tf-test | notes |
|---|---|---|---|---|---|
| CreateCertificate | `POST /certificates/{name}/create` | ✓ `keyvault.go::handleKVCreateCertificate` | ✗ | ✗ | Currently returns 200 + Certificate JSON; **real Azure returns 202 + CertificateOperation** with `status:"inProgress"`. SDK can't fully round-trip Create today (filed for follow-up). |
| GetCertificate | `GET /certificates/{name}` | ✓ `handleKVGetCertificate` | ✓ `TestKeyVault_SDK_Certificates_ChallengeRoundTrip` | ✗ | tf: `azurerm_key_vault_certificate` |
| ListCertificates | `GET /certificates` | ✓ `handleKVListCertificates` | ✗ | ✗ | |
| DeleteCertificate | `DELETE /certificates/{name}` | ✓ `handleKVDeleteCertificate` | ✗ | ✗ | |
| GetCertificateOperation | `GET /certificates/{name}/pending` | ✗ | ✗ | ✗ | LRO poll endpoint. Needs CreateCertificate to return 202 first. |
| UpdateCertificate | `PATCH /certificates/{name}/{version}` | ✗ | ✗ | ✗ | |
| ImportCertificate | `POST /certificates/{name}/import` | ✗ | ✗ | ✗ | |
| MergeCertificate | `POST /certificates/{name}/pending/merge` | ✗ | ✗ | ✗ | |

## Open subtasks staged forward

- CreateCertificate response shape: switch to canonical 202 + CertificateOperation + add `GET /certificates/{name}/pending` polling endpoint so the full SDK LRO contract round-trips. File as follow-up BUG when a runner scenario lands.
- Cryptographic operations (Sign/Verify/Encrypt/Decrypt/WrapKey/UnwrapKey): out of scope for the simulator (no real key material), but should surface as canonical 501 NotImplemented envelopes rather than 404 fall-throughs.
- tf-tests for `azurerm_key_vault_secret` covered under BUG-1147 in Phase 177; `azurerm_key_vault_key` + `azurerm_key_vault_certificate` deferred until a runner scenario lands.
- **All remaining ✗ sim-handler rows** (BackupSecret/RestoreSecret, UpdateKey, ImportKey, crypto ops, BackupKey/RestoreKey, GetCertificateOperation, UpdateCertificate, ImportCertificate, MergeCertificate) are deferred under this entry. Backup/Restore and crypto operations are big surfaces requiring their own persistence or key-material model; UpdateKey / UpdateCertificate are the most likely next requests from a community-filed issue and land first when one arrives.
- **All remaining ✗ sdk-test rows** (ListSecrets, ListKeys, ListKeyVersions, DeleteKey, ListCertificates, DeleteCertificate, UpdateSecret) are deferred under this entry. The sim handlers exist for several of these rows; the remaining gap is canonical-SDK-driven test coverage. Sweep lands when a community-filed issue or scheduled audit surfaces a regression.

## Reopens that produced this table

- Issue [#193](https://github.com/e6qu/sockerless/issues/193) reopened — PR #200's `WWW-Authenticate` URL had 3 path segments; Azure SDKs panicked at `parts[3]`. PR #200's coverage test used raw `net/http` + `Authorization: Bearer fake-token`, which bypassed the challenge flow entirely and never exercised the SDK's parser. This table makes the gap visible: every KV op that has an SDK client (Secrets / Keys / Certificates) is held to having an SDK-driven sdk-test.
