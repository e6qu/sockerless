# Sim surface — aws-iam

Surface registered in `simulators/aws/iam.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action CreateRole` | ✓ `simulators/aws/iam.go:73::handleIAMCreateRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetRole` | ✓ `simulators/aws/iam.go:74::handleIAMGetRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRole` | ✓ `simulators/aws/iam.go:75::handleIAMDeleteRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action UpdateAssumeRolePolicy` | ✓ `simulators/aws/iam.go:76::handleIAMUpdateAssumeRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action PutRolePolicy` | ✓ `simulators/aws/iam.go:77::handleIAMPutRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetRolePolicy` | ✓ `simulators/aws/iam.go:78::handleIAMGetRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteRolePolicy` | ✓ `simulators/aws/iam.go:79::handleIAMDeleteRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AttachRolePolicy` | ✓ `simulators/aws/iam.go:80::handleIAMAttachRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DetachRolePolicy` | ✓ `simulators/aws/iam.go:81::handleIAMDetachRolePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListAttachedRolePolicies` | ✓ `simulators/aws/iam.go:82::handleIAMListAttachedRolePolicies` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListRolePolicies` | ✓ `simulators/aws/iam.go:83::handleIAMListRolePolicies` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListInstanceProfilesForRole` | ✓ `simulators/aws/iam.go:84::handleIAMListInstanceProfilesForRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreatePolicy` | ✓ `simulators/aws/iam.go:88::handleIAMCreatePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetPolicy` | ✓ `simulators/aws/iam.go:89::handleIAMGetPolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeletePolicy` | ✓ `simulators/aws/iam.go:90::handleIAMDeletePolicy` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListPolicies` | ✓ `simulators/aws/iam.go:91::handleIAMListPolicies` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetPolicyVersion` | ✓ `simulators/aws/iam.go:92::handleIAMGetPolicyVersion` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateInstanceProfile` | ✓ `simulators/aws/iam.go:97::handleIAMCreateInstanceProfile` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetInstanceProfile` | ✓ `simulators/aws/iam.go:98::handleIAMGetInstanceProfile` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteInstanceProfile` | ✓ `simulators/aws/iam.go:99::handleIAMDeleteInstanceProfile` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListInstanceProfiles` | ✓ `simulators/aws/iam.go:100::handleIAMListInstanceProfiles` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AddRoleToInstanceProfile` | ✓ `simulators/aws/iam.go:101::handleIAMAddRoleToInstanceProfile` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RemoveRoleFromInstanceProfile` | ✓ `simulators/aws/iam.go:102::handleIAMRemoveRoleFromInstanceProfile` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:62::handleIAMCreateServiceLinkedRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:63::handleIAMDeleteServiceLinkedRole` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetServiceLinkedRoleDeletionStatus` | ✓ `simulators/aws/iam_slr_oidc.go:64::handleIAMGetSLRDeletionStatus` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action CreateOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:66::handleIAMCreateOIDCProvider` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action GetOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:67::handleIAMGetOIDCProvider` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action UpdateOpenIDConnectProviderThumbprint` | ✓ `simulators/aws/iam_slr_oidc.go:68::handleIAMUpdateOIDCThumbprint` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AddClientIDToOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:69::handleIAMAddOIDCClientID` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action RemoveClientIDFromOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:70::handleIAMRemoveOIDCClientID` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action DeleteOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:71::handleIAMDeleteOIDCProvider` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action ListOpenIDConnectProviders` | ✓ `simulators/aws/iam_slr_oidc.go:72::handleIAMListOIDCProviders` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
