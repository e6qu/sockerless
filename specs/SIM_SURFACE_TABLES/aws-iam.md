# Sim surface — aws-iam

Surface registered in `simulators/aws/iam.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateRole` | ✓ `simulators/aws/iam.go:73::handleIAMCreateRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetRole` | ✓ `simulators/aws/iam.go:74::handleIAMGetRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRole` | ✓ `simulators/aws/iam.go:75::handleIAMDeleteRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateAssumeRolePolicy` | ✓ `simulators/aws/iam.go:76::handleIAMUpdateAssumeRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutRolePolicy` | ✓ `simulators/aws/iam.go:77::handleIAMPutRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetRolePolicy` | ✓ `simulators/aws/iam.go:78::handleIAMGetRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRolePolicy` | ✓ `simulators/aws/iam.go:79::handleIAMDeleteRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AttachRolePolicy` | ✓ `simulators/aws/iam.go:80::handleIAMAttachRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DetachRolePolicy` | ✓ `simulators/aws/iam.go:81::handleIAMDetachRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAttachedRolePolicies` | ✓ `simulators/aws/iam.go:82::handleIAMListAttachedRolePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListRolePolicies` | ✓ `simulators/aws/iam.go:83::handleIAMListRolePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListInstanceProfilesForRole` | ✓ `simulators/aws/iam.go:84::handleIAMListInstanceProfilesForRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreatePolicy` | ✓ `simulators/aws/iam.go:88::handleIAMCreatePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetPolicy` | ✓ `simulators/aws/iam.go:89::handleIAMGetPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeletePolicy` | ✓ `simulators/aws/iam.go:90::handleIAMDeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPolicies` | ✓ `simulators/aws/iam.go:91::handleIAMListPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetPolicyVersion` | ✓ `simulators/aws/iam.go:92::handleIAMGetPolicyVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateInstanceProfile` | ✓ `simulators/aws/iam.go:97::handleIAMCreateInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetInstanceProfile` | ✓ `simulators/aws/iam.go:98::handleIAMGetInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteInstanceProfile` | ✓ `simulators/aws/iam.go:99::handleIAMDeleteInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListInstanceProfiles` | ✓ `simulators/aws/iam.go:100::handleIAMListInstanceProfiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddRoleToInstanceProfile` | ✓ `simulators/aws/iam.go:101::handleIAMAddRoleToInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveRoleFromInstanceProfile` | ✓ `simulators/aws/iam.go:102::handleIAMRemoveRoleFromInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:62::handleIAMCreateServiceLinkedRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:63::handleIAMDeleteServiceLinkedRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetServiceLinkedRoleDeletionStatus` | ✓ `simulators/aws/iam_slr_oidc.go:64::handleIAMGetSLRDeletionStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:66::handleIAMCreateOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:67::handleIAMGetOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateOpenIDConnectProviderThumbprint` | ✓ `simulators/aws/iam_slr_oidc.go:68::handleIAMUpdateOIDCThumbprint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddClientIDToOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:69::handleIAMAddOIDCClientID` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveClientIDFromOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:70::handleIAMRemoveOIDCClientID` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:71::handleIAMDeleteOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListOpenIDConnectProviders` | ✓ `simulators/aws/iam_slr_oidc.go:72::handleIAMListOIDCProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
