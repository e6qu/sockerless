# Sim surface — aws-wafv2

Surface registered in `simulators/aws/wafv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSWAF_20190729.CreateWebACL` | ✓ `simulators/aws/wafv2.go:227::handleWAFCreateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetWebACL` | ✓ `simulators/aws/wafv2.go:228::handleWAFGetWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateWebACL` | ✓ `simulators/aws/wafv2.go:229::handleWAFUpdateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteWebACL` | ✓ `simulators/aws/wafv2.go:230::handleWAFDeleteWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListWebACLs` | ✓ `simulators/aws/wafv2.go:231::handleWAFListWebACLs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.AssociateWebACL` | ✓ `simulators/aws/wafv2.go:233::handleWAFAssociateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DisassociateWebACL` | ✓ `simulators/aws/wafv2.go:234::handleWAFDisassociateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetWebACLForResource` | ✓ `simulators/aws/wafv2.go:235::handleWAFGetWebACLForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListResourcesForWebACL` | ✓ `simulators/aws/wafv2.go:236::handleWAFListResourcesForWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateIPSet` | ✓ `simulators/aws/wafv2.go:238::handleWAFCreateIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetIPSet` | ✓ `simulators/aws/wafv2.go:239::handleWAFGetIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateIPSet` | ✓ `simulators/aws/wafv2.go:240::handleWAFUpdateIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteIPSet` | ✓ `simulators/aws/wafv2.go:241::handleWAFDeleteIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListIPSets` | ✓ `simulators/aws/wafv2.go:242::handleWAFListIPSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateRuleGroup` | ✓ `simulators/aws/wafv2.go:244::handleWAFCreateRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRuleGroup` | ✓ `simulators/aws/wafv2.go:245::handleWAFGetRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateRuleGroup` | ✓ `simulators/aws/wafv2.go:246::handleWAFUpdateRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteRuleGroup` | ✓ `simulators/aws/wafv2.go:247::handleWAFDeleteRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListRuleGroups` | ✓ `simulators/aws/wafv2.go:248::handleWAFListRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:250::handleWAFCreateRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRegexPatternSet` | ✓ `simulators/aws/wafv2.go:251::handleWAFGetRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:252::handleWAFUpdateRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteRegexPatternSet` | ✓ `simulators/aws/wafv2.go:253::handleWAFDeleteRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListRegexPatternSets` | ✓ `simulators/aws/wafv2.go:254::handleWAFListRegexSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.TagResource` | ✓ `simulators/aws/wafv2.go:256::handleWAFTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UntagResource` | ✓ `simulators/aws/wafv2.go:257::handleWAFUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListTagsForResource` | ✓ `simulators/aws/wafv2.go:258::handleWAFListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:260::handleWAFPutLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:261::handleWAFGetLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:262::handleWAFDeleteLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListLoggingConfigurations` | ✓ `simulators/aws/wafv2.go:263::handleWAFListLoggingConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetSampledRequests` | ✓ `simulators/aws/wafv2.go:265::handleWAFGetSampledRequests` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateAPIKey` | ✓ `simulators/aws/wafv2.go:267::handleWAFCreateAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteAPIKey` | ✓ `simulators/aws/wafv2.go:268::handleWAFDeleteAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAPIKeys` | ✓ `simulators/aws/wafv2.go:269::handleWAFListAPIKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetDecryptedAPIKey` | ✓ `simulators/aws/wafv2.go:270::handleWAFGetDecryptedAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CheckCapacity` | ✓ `simulators/aws/wafv2.go:272::handleWAFCheckCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeManagedRuleGroup` | ✓ `simulators/aws/wafv2.go:274::handleWAFDescribeManagedRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeAllManagedProducts` | ✓ `simulators/aws/wafv2.go:275::handleWAFDescribeAllManagedProducts` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeManagedProductsByVendor` | ✓ `simulators/aws/wafv2.go:276::handleWAFDescribeManagedProductsByVendor` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAvailableManagedRuleGroups` | ✓ `simulators/aws/wafv2.go:277::handleWAFListAvailableManagedRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAvailableManagedRuleGroupVersions` | ✓ `simulators/aws/wafv2.go:278::handleWAFListAvailableManagedRuleGroupVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetManagedRuleSet` | ✓ `simulators/aws/wafv2.go:280::handleWAFGetManagedRuleSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListManagedRuleSets` | ✓ `simulators/aws/wafv2.go:281::handleWAFListManagedRuleSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutManagedRuleSetVersions` | ✓ `simulators/aws/wafv2.go:282::handleWAFPutManagedRuleSetVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateManagedRuleSetVersionExpiryDate` | ✓ `simulators/aws/wafv2.go:283::handleWAFUpdateManagedRuleSetVersionExpiryDate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutPermissionPolicy` | ✓ `simulators/aws/wafv2.go:285::handleWAFPutPermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetPermissionPolicy` | ✓ `simulators/aws/wafv2.go:286::handleWAFGetPermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeletePermissionPolicy` | ✓ `simulators/aws/wafv2.go:287::handleWAFDeletePermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GenerateMobileSdkReleaseUrl` | ✓ `simulators/aws/wafv2.go:289::handleWAFGenerateMobileSdkReleaseUrl` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetMobileSdkRelease` | ✓ `simulators/aws/wafv2.go:290::handleWAFGetMobileSdkRelease` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListMobileSdkReleases` | ✓ `simulators/aws/wafv2.go:291::handleWAFListMobileSdkReleases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteFirewallManagerRuleGroups` | ✓ `simulators/aws/wafv2.go:293::handleWAFDeleteFirewallManagerRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRateBasedStatementManagedKeys` | ✓ `simulators/aws/wafv2.go:295::handleWAFGetRateBasedStatementManagedKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetTopPathStatisticsByTraffic` | ✓ `simulators/aws/wafv2.go:296::handleWAFGetTopPathStatisticsByTraffic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
