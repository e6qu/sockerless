# Sim surface — aws-kms

Surface registered in `simulators/aws/kms_crypto.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action TrentService.Sign` | ✓ `simulators/aws/kms_crypto.go:45::handleKMSSign` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.Verify` | ✓ `simulators/aws/kms_crypto.go:46::handleKMSVerify` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetPublicKey` | ✓ `simulators/aws/kms_crypto.go:47::handleKMSGetPublicKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateMac` | ✓ `simulators/aws/kms_crypto.go:48::handleKMSGenerateMac` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.VerifyMac` | ✓ `simulators/aws/kms_crypto.go:49::handleKMSVerifyMac` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateDataKeyPair` | ✓ `simulators/aws/kms_crypto.go:50::handleKMSGenerateDataKeyPair` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateDataKeyPairWithoutPlaintext` | ✓ `simulators/aws/kms_crypto.go:51::handleKMSGenerateDataKeyPairWithoutPlaintext` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DeriveSharedSecret` | ✓ `simulators/aws/kms_crypto.go:52::handleKMSDeriveSharedSecret` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CreateCustomKeyStore` | ✓ `simulators/aws/kms_custom_key_stores.go:31::handleKMSCreateCustomKeyStore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DescribeCustomKeyStores` | ✓ `simulators/aws/kms_custom_key_stores.go:32::handleKMSDescribeCustomKeyStores` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ConnectCustomKeyStore` | ✓ `simulators/aws/kms_custom_key_stores.go:33::handleKMSConnectCustomKeyStore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DisconnectCustomKeyStore` | ✓ `simulators/aws/kms_custom_key_stores.go:34::handleKMSDisconnectCustomKeyStore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.UpdateCustomKeyStore` | ✓ `simulators/aws/kms_custom_key_stores.go:35::handleKMSUpdateCustomKeyStore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DeleteCustomKeyStore` | ✓ `simulators/aws/kms_custom_key_stores.go:36::handleKMSDeleteCustomKeyStore` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CreateGrant` | ✓ `simulators/aws/kms_grants.go:35::handleKMSCreateGrant` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListGrants` | ✓ `simulators/aws/kms_grants.go:36::handleKMSListGrants` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.RevokeGrant` | ✓ `simulators/aws/kms_grants.go:37::handleKMSRevokeGrant` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateDataKeyWithoutPlaintext` | ✓ `simulators/aws/kms_grants.go:38::handleKMSGenerateDataKeyWithoutPlaintext` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ReEncrypt` | ✓ `simulators/aws/kms_grants.go:39::handleKMSReEncrypt` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.RetireGrant` | ✓ `simulators/aws/kms_multiregion.go:17::handleKMSRetireGrant` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListRetirableGrants` | ✓ `simulators/aws/kms_multiregion.go:18::handleKMSListRetirableGrants` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ReplicateKey` | ✓ `simulators/aws/kms_multiregion.go:19::handleKMSReplicateKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.UpdatePrimaryRegion` | ✓ `simulators/aws/kms_multiregion.go:20::handleKMSUpdatePrimaryRegion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetKeyLastUsage` | ✓ `simulators/aws/kms_multiregion.go:21::handleKMSGetKeyLastUsage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CreateKey` | ✓ `simulators/aws/kms.go:132::handleKMSCreateKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DescribeKey` | ✓ `simulators/aws/kms.go:133::handleKMSDescribeKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListKeys` | ✓ `simulators/aws/kms.go:134::handleKMSListKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ScheduleKeyDeletion` | ✓ `simulators/aws/kms.go:135::handleKMSScheduleKeyDeletion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.Encrypt` | ✓ `simulators/aws/kms.go:136::handleKMSEncrypt` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.Decrypt` | ✓ `simulators/aws/kms.go:137::handleKMSDecrypt` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateDataKey` | ✓ `simulators/aws/kms.go:138::handleKMSGenerateDataKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CreateAlias` | ✓ `simulators/aws/kms.go:139::handleKMSCreateAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DeleteAlias` | ✓ `simulators/aws/kms.go:140::handleKMSDeleteAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListAliases` | ✓ `simulators/aws/kms.go:141::handleKMSListAliases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetKeyPolicy` | ✓ `simulators/aws/kms.go:142::handleKMSGetKeyPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.PutKeyPolicy` | ✓ `simulators/aws/kms.go:143::handleKMSPutKeyPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListResourceTags` | ✓ `simulators/aws/kms.go:144::handleKMSListResourceTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.TagResource` | ✓ `simulators/aws/kms.go:145::handleKMSTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.UntagResource` | ✓ `simulators/aws/kms.go:146::handleKMSUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetKeyRotationStatus` | ✓ `simulators/aws/kms.go:147::handleKMSGetKeyRotationStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.EnableKeyRotation` | ✓ `simulators/aws/kms.go:148::handleKMSEnableKeyRotation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DisableKeyRotation` | ✓ `simulators/aws/kms.go:149::handleKMSDisableKeyRotation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.EnableKey` | ✓ `simulators/aws/kms.go:151::handleKMSEnableKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DisableKey` | ✓ `simulators/aws/kms.go:152::handleKMSDisableKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.CancelKeyDeletion` | ✓ `simulators/aws/kms.go:153::handleKMSCancelKeyDeletion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.UpdateKeyDescription` | ✓ `simulators/aws/kms.go:154::handleKMSUpdateKeyDescription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.UpdateAlias` | ✓ `simulators/aws/kms.go:155::handleKMSUpdateAlias` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GenerateRandom` | ✓ `simulators/aws/kms.go:156::handleKMSGenerateRandom` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListKeyPolicies` | ✓ `simulators/aws/kms.go:157::handleKMSListKeyPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ListKeyRotations` | ✓ `simulators/aws/kms.go:158::handleKMSListKeyRotations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.RotateKeyOnDemand` | ✓ `simulators/aws/kms.go:159::handleKMSRotateKeyOnDemand` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.GetParametersForImport` | ✓ `simulators/aws/kms.go:160::handleKMSGetParametersForImport` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.ImportKeyMaterial` | ✓ `simulators/aws/kms.go:161::handleKMSImportKeyMaterial` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TrentService.DeleteImportedKeyMaterial` | ✓ `simulators/aws/kms.go:162::handleKMSDeleteImportedKeyMaterial` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
