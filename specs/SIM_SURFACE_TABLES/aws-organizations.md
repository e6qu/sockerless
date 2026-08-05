# Sim surface — aws-organizations

Surface registered in `simulators/aws/organizations.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSOrganizationsV20161128.CreateOrganization` | ✓ `simulators/aws/organizations.go:194::handleOrgCreateOrganization` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeleteOrganization` | ✓ `simulators/aws/organizations.go:195::handleOrgDeleteOrganization` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeOrganization` | ✓ `simulators/aws/organizations.go:196::handleOrgDescribeOrganization` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.EnableAllFeatures` | ✓ `simulators/aws/organizations.go:197::handleOrgEnableAllFeatures` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.CreateAccount` | ✓ `simulators/aws/organizations.go:200::handleOrgCreateAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeAccount` | ✓ `simulators/aws/organizations.go:201::handleOrgDescribeAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeCreateAccountStatus` | ✓ `simulators/aws/organizations.go:202::handleOrgDescribeCreateAccountStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListCreateAccountStatus` | ✓ `simulators/aws/organizations.go:203::handleOrgListCreateAccountStatus` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListAccounts` | ✓ `simulators/aws/organizations.go:204::handleOrgListAccounts` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListAccountsForParent` | ✓ `simulators/aws/organizations.go:205::handleOrgListAccountsForParent` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.MoveAccount` | ✓ `simulators/aws/organizations.go:206::handleOrgMoveAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.RemoveAccountFromOrganization` | ✓ `simulators/aws/organizations.go:207::handleOrgRemoveAccountFromOrganization` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.CloseAccount` | ✓ `simulators/aws/organizations.go:208::handleOrgCloseAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.CreateOrganizationalUnit` | ✓ `simulators/aws/organizations.go:211::handleOrgCreateOU` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeleteOrganizationalUnit` | ✓ `simulators/aws/organizations.go:212::handleOrgDeleteOU` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeOrganizationalUnit` | ✓ `simulators/aws/organizations.go:213::handleOrgDescribeOU` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.UpdateOrganizationalUnit` | ✓ `simulators/aws/organizations.go:214::handleOrgUpdateOU` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListOrganizationalUnitsForParent` | ✓ `simulators/aws/organizations.go:215::handleOrgListOUsForParent` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListRoots` | ✓ `simulators/aws/organizations.go:218::handleOrgListRoots` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListChildren` | ✓ `simulators/aws/organizations.go:219::handleOrgListChildren` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListParents` | ✓ `simulators/aws/organizations.go:220::handleOrgListParents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.CreatePolicy` | ✓ `simulators/aws/organizations.go:223::handleOrgCreatePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeletePolicy` | ✓ `simulators/aws/organizations.go:224::handleOrgDeletePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribePolicy` | ✓ `simulators/aws/organizations.go:225::handleOrgDescribePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.UpdatePolicy` | ✓ `simulators/aws/organizations.go:226::handleOrgUpdatePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.AttachPolicy` | ✓ `simulators/aws/organizations.go:227::handleOrgAttachPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DetachPolicy` | ✓ `simulators/aws/organizations.go:228::handleOrgDetachPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListPolicies` | ✓ `simulators/aws/organizations.go:229::handleOrgListPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListPoliciesForTarget` | ✓ `simulators/aws/organizations.go:230::handleOrgListPoliciesForTarget` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListTargetsForPolicy` | ✓ `simulators/aws/organizations.go:231::handleOrgListTargetsForPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.EnablePolicyType` | ✓ `simulators/aws/organizations.go:232::handleOrgEnablePolicyType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DisablePolicyType` | ✓ `simulators/aws/organizations.go:233::handleOrgDisablePolicyType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeEffectivePolicy` | ✓ `simulators/aws/organizations.go:234::handleOrgDescribeEffectivePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.InviteAccountToOrganization` | ✓ `simulators/aws/organizations.go:237::handleOrgInviteAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.AcceptHandshake` | ✓ `simulators/aws/organizations.go:238::handleOrgAcceptHandshake` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeclineHandshake` | ✓ `simulators/aws/organizations.go:239::handleOrgDeclineHandshake` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.CancelHandshake` | ✓ `simulators/aws/organizations.go:240::handleOrgCancelHandshake` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeHandshake` | ✓ `simulators/aws/organizations.go:241::handleOrgDescribeHandshake` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListHandshakesForAccount` | ✓ `simulators/aws/organizations.go:242::handleOrgListHandshakesForAccount` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListHandshakesForOrganization` | ✓ `simulators/aws/organizations.go:243::handleOrgListHandshakesForOrganization` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.RegisterDelegatedAdministrator` | ✓ `simulators/aws/organizations.go:246::handleOrgRegisterDelegatedAdmin` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeregisterDelegatedAdministrator` | ✓ `simulators/aws/organizations.go:247::handleOrgDeregisterDelegatedAdmin` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListDelegatedAdministrators` | ✓ `simulators/aws/organizations.go:248::handleOrgListDelegatedAdmins` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListDelegatedServicesForAccount` | ✓ `simulators/aws/organizations.go:249::handleOrgListDelegatedServices` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.EnableAWSServiceAccess` | ✓ `simulators/aws/organizations.go:250::handleOrgEnableServiceAccess` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DisableAWSServiceAccess` | ✓ `simulators/aws/organizations.go:251::handleOrgDisableServiceAccess` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListAWSServiceAccessForOrganization` | ✓ `simulators/aws/organizations.go:252::handleOrgListServiceAccess` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.PutResourcePolicy` | ✓ `simulators/aws/organizations.go:255::handleOrgPutResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DeleteResourcePolicy` | ✓ `simulators/aws/organizations.go:256::handleOrgDeleteResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.DescribeResourcePolicy` | ✓ `simulators/aws/organizations.go:257::handleOrgDescribeResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.TagResource` | ✓ `simulators/aws/organizations.go:260::handleOrgTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.UntagResource` | ✓ `simulators/aws/organizations.go:261::handleOrgUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSOrganizationsV20161128.ListTagsForResource` | ✓ `simulators/aws/organizations.go:262::handleOrgListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
