# Sim surface — aws-iam

Surface registered in `simulators/aws/iam_groups.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action PutUserPermissionsBoundary` | ✓ `simulators/aws/iam_groups.go:64::handleIAMPutUserBoundary` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteUserPermissionsBoundary` | ✓ `simulators/aws/iam_groups.go:65::handleIAMDeleteUserBoundary` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateGroup` | ✓ `simulators/aws/iam_groups.go:67::handleIAMCreateGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetGroup` | ✓ `simulators/aws/iam_groups.go:68::handleIAMGetGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteGroup` | ✓ `simulators/aws/iam_groups.go:69::handleIAMDeleteGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListGroups` | ✓ `simulators/aws/iam_groups.go:70::handleIAMListGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddUserToGroup` | ✓ `simulators/aws/iam_groups.go:71::handleIAMAddUserToGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveUserFromGroup` | ✓ `simulators/aws/iam_groups.go:72::handleIAMRemoveUserFromGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListGroupsForUser` | ✓ `simulators/aws/iam_groups.go:73::handleIAMListGroupsForUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutGroupPolicy` | ✓ `simulators/aws/iam_groups.go:74::handleIAMPutGroupPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetGroupPolicy` | ✓ `simulators/aws/iam_groups.go:75::handleIAMGetGroupPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteGroupPolicy` | ✓ `simulators/aws/iam_groups.go:76::handleIAMDeleteGroupPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListGroupPolicies` | ✓ `simulators/aws/iam_groups.go:77::handleIAMListGroupPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AttachGroupPolicy` | ✓ `simulators/aws/iam_groups.go:78::handleIAMAttachGroupPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DetachGroupPolicy` | ✓ `simulators/aws/iam_groups.go:79::handleIAMDetachGroupPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAttachedGroupPolicies` | ✓ `simulators/aws/iam_groups.go:80::handleIAMListAttachedGroupPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListUsers` | ✓ `simulators/aws/iam_groups.go:81::handleIAMListUsers` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPolicyVersions` | ✓ `simulators/aws/iam_lists.go:28::handleIAMListPolicyVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListRoles` | ✓ `simulators/aws/iam_lists.go:29::handleIAMListRoles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListRoleTags` | ✓ `simulators/aws/iam_lists.go:30::handleIAMListRoleTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPolicyTags` | ✓ `simulators/aws/iam_lists.go:31::handleIAMListPolicyTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SimulateCustomPolicy` | ✓ `simulators/aws/iam_policy_sim.go:730::handleIAMSimulateCustomPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SimulatePrincipalPolicy` | ✓ `simulators/aws/iam_policy_sim.go:731::handleIAMSimulatePrincipalPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:63::handleIAMCreateServiceLinkedRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteServiceLinkedRole` | ✓ `simulators/aws/iam_slr_oidc.go:64::handleIAMDeleteServiceLinkedRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetServiceLinkedRoleDeletionStatus` | ✓ `simulators/aws/iam_slr_oidc.go:65::handleIAMGetSLRDeletionStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:67::handleIAMCreateOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:68::handleIAMGetOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TagOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:69::handleIAMTagOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UntagOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:70::handleIAMUntagOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateOpenIDConnectProviderThumbprint` | ✓ `simulators/aws/iam_slr_oidc.go:71::handleIAMUpdateOIDCThumbprint` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddClientIDToOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:72::handleIAMAddOIDCClientID` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveClientIDFromOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:73::handleIAMRemoveOIDCClientID` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteOpenIDConnectProvider` | ✓ `simulators/aws/iam_slr_oidc.go:74::handleIAMDeleteOIDCProvider` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListOpenIDConnectProviders` | ✓ `simulators/aws/iam_slr_oidc.go:75::handleIAMListOIDCProviders` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateUser` | ✓ `simulators/aws/iam_users.go:63::handleIAMCreateUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetUser` | ✓ `simulators/aws/iam_users.go:64::handleIAMGetUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteUser` | ✓ `simulators/aws/iam_users.go:65::handleIAMDeleteUser` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateAccessKey` | ✓ `simulators/aws/iam_users.go:66::handleIAMCreateAccessKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAccessKey` | ✓ `simulators/aws/iam_users.go:67::handleIAMDeleteAccessKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAccessKeys` | ✓ `simulators/aws/iam_users.go:68::handleIAMListAccessKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutUserPolicy` | ✓ `simulators/aws/iam_users.go:69::handleIAMPutUserPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetUserPolicy` | ✓ `simulators/aws/iam_users.go:70::handleIAMGetUserPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteUserPolicy` | ✓ `simulators/aws/iam_users.go:71::handleIAMDeleteUserPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListUserPolicies` | ✓ `simulators/aws/iam_users.go:72::handleIAMListUserPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AttachUserPolicy` | ✓ `simulators/aws/iam_users.go:73::handleIAMAttachUserPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DetachUserPolicy` | ✓ `simulators/aws/iam_users.go:74::handleIAMDetachUserPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAttachedUserPolicies` | ✓ `simulators/aws/iam_users.go:75::handleIAMListAttachedUserPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateRole` | ✓ `simulators/aws/iam.go:78::handleIAMCreateRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetRole` | ✓ `simulators/aws/iam.go:79::handleIAMGetRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRole` | ✓ `simulators/aws/iam.go:80::handleIAMDeleteRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateRole` | ✓ `simulators/aws/iam.go:81::handleIAMUpdateRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TagRole` | ✓ `simulators/aws/iam.go:82::handleIAMTagRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UntagRole` | ✓ `simulators/aws/iam.go:83::handleIAMUntagRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UpdateAssumeRolePolicy` | ✓ `simulators/aws/iam.go:84::handleIAMUpdateAssumeRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutRolePolicy` | ✓ `simulators/aws/iam.go:85::handleIAMPutRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetRolePolicy` | ✓ `simulators/aws/iam.go:86::handleIAMGetRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteRolePolicy` | ✓ `simulators/aws/iam.go:87::handleIAMDeleteRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AttachRolePolicy` | ✓ `simulators/aws/iam.go:88::handleIAMAttachRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DetachRolePolicy` | ✓ `simulators/aws/iam.go:89::handleIAMDetachRolePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAttachedRolePolicies` | ✓ `simulators/aws/iam.go:90::handleIAMListAttachedRolePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListRolePolicies` | ✓ `simulators/aws/iam.go:91::handleIAMListRolePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListInstanceProfilesForRole` | ✓ `simulators/aws/iam.go:92::handleIAMListInstanceProfilesForRole` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreatePolicy` | ✓ `simulators/aws/iam.go:96::handleIAMCreatePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetPolicy` | ✓ `simulators/aws/iam.go:97::handleIAMGetPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeletePolicy` | ✓ `simulators/aws/iam.go:98::handleIAMDeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListPolicies` | ✓ `simulators/aws/iam.go:99::handleIAMListPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetPolicyVersion` | ✓ `simulators/aws/iam.go:100::handleIAMGetPolicyVersion` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action CreateInstanceProfile` | ✓ `simulators/aws/iam.go:105::handleIAMCreateInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetInstanceProfile` | ✓ `simulators/aws/iam.go:106::handleIAMGetInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteInstanceProfile` | ✓ `simulators/aws/iam.go:107::handleIAMDeleteInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListInstanceProfiles` | ✓ `simulators/aws/iam.go:108::handleIAMListInstanceProfiles` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AddRoleToInstanceProfile` | ✓ `simulators/aws/iam.go:109::handleIAMAddRoleToInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action RemoveRoleFromInstanceProfile` | ✓ `simulators/aws/iam.go:110::handleIAMRemoveRoleFromInstanceProfile` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
