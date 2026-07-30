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
| `Action AWSWAF_20190729.CreateWebACL` | ✓ `simulators/aws/wafv2.go:260::handleWAFCreateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetWebACL` | ✓ `simulators/aws/wafv2.go:261::handleWAFGetWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateWebACL` | ✓ `simulators/aws/wafv2.go:262::handleWAFUpdateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteWebACL` | ✓ `simulators/aws/wafv2.go:263::handleWAFDeleteWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListWebACLs` | ✓ `simulators/aws/wafv2.go:264::handleWAFListWebACLs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.AssociateWebACL` | ✓ `simulators/aws/wafv2.go:266::handleWAFAssociateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DisassociateWebACL` | ✓ `simulators/aws/wafv2.go:267::handleWAFDisassociateWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetWebACLForResource` | ✓ `simulators/aws/wafv2.go:268::handleWAFGetWebACLForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListResourcesForWebACL` | ✓ `simulators/aws/wafv2.go:269::handleWAFListResourcesForWebACL` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateIPSet` | ✓ `simulators/aws/wafv2.go:271::handleWAFCreateIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetIPSet` | ✓ `simulators/aws/wafv2.go:272::handleWAFGetIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateIPSet` | ✓ `simulators/aws/wafv2.go:273::handleWAFUpdateIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteIPSet` | ✓ `simulators/aws/wafv2.go:274::handleWAFDeleteIPSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListIPSets` | ✓ `simulators/aws/wafv2.go:275::handleWAFListIPSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateRuleGroup` | ✓ `simulators/aws/wafv2.go:277::handleWAFCreateRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRuleGroup` | ✓ `simulators/aws/wafv2.go:278::handleWAFGetRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateRuleGroup` | ✓ `simulators/aws/wafv2.go:279::handleWAFUpdateRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteRuleGroup` | ✓ `simulators/aws/wafv2.go:280::handleWAFDeleteRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListRuleGroups` | ✓ `simulators/aws/wafv2.go:281::handleWAFListRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:283::handleWAFCreateRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRegexPatternSet` | ✓ `simulators/aws/wafv2.go:284::handleWAFGetRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:285::handleWAFUpdateRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteRegexPatternSet` | ✓ `simulators/aws/wafv2.go:286::handleWAFDeleteRegexSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListRegexPatternSets` | ✓ `simulators/aws/wafv2.go:287::handleWAFListRegexSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.TagResource` | ✓ `simulators/aws/wafv2.go:289::handleWAFTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UntagResource` | ✓ `simulators/aws/wafv2.go:290::handleWAFUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListTagsForResource` | ✓ `simulators/aws/wafv2.go:291::handleWAFListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:293::handleWAFPutLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:294::handleWAFGetLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteLoggingConfiguration` | ✓ `simulators/aws/wafv2.go:295::handleWAFDeleteLoggingConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListLoggingConfigurations` | ✓ `simulators/aws/wafv2.go:296::handleWAFListLoggingConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetSampledRequests` | ✓ `simulators/aws/wafv2.go:298::handleWAFGetSampledRequests` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRevenueStatistics` | ✓ `simulators/aws/wafv2.go:299::handleWAFGetRevenueStatistics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRevenueStatisticsSummary` | ✓ `simulators/aws/wafv2.go:300::handleWAFGetRevenueStatisticsSummary` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRevenueStatisticsTimeSeries` | ✓ `simulators/aws/wafv2.go:301::handleWAFGetRevenueStatisticsTimeSeries` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListSettlementRecords` | ✓ `simulators/aws/wafv2.go:302::handleWAFListSettlementRecords` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CreateAPIKey` | ✓ `simulators/aws/wafv2.go:304::handleWAFCreateAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteAPIKey` | ✓ `simulators/aws/wafv2.go:305::handleWAFDeleteAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAPIKeys` | ✓ `simulators/aws/wafv2.go:306::handleWAFListAPIKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetDecryptedAPIKey` | ✓ `simulators/aws/wafv2.go:307::handleWAFGetDecryptedAPIKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.CheckCapacity` | ✓ `simulators/aws/wafv2.go:309::handleWAFCheckCapacity` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeManagedRuleGroup` | ✓ `simulators/aws/wafv2.go:311::handleWAFDescribeManagedRuleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeAllManagedProducts` | ✓ `simulators/aws/wafv2.go:312::handleWAFDescribeAllManagedProducts` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DescribeManagedProductsByVendor` | ✓ `simulators/aws/wafv2.go:313::handleWAFDescribeManagedProductsByVendor` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAvailableManagedRuleGroups` | ✓ `simulators/aws/wafv2.go:314::handleWAFListAvailableManagedRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListAvailableManagedRuleGroupVersions` | ✓ `simulators/aws/wafv2.go:315::handleWAFListAvailableManagedRuleGroupVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetManagedRuleSet` | ✓ `simulators/aws/wafv2.go:317::handleWAFGetManagedRuleSet` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListManagedRuleSets` | ✓ `simulators/aws/wafv2.go:318::handleWAFListManagedRuleSets` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutManagedRuleSetVersions` | ✓ `simulators/aws/wafv2.go:319::handleWAFPutManagedRuleSetVersions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.UpdateManagedRuleSetVersionExpiryDate` | ✓ `simulators/aws/wafv2.go:320::handleWAFUpdateManagedRuleSetVersionExpiryDate` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.PutPermissionPolicy` | ✓ `simulators/aws/wafv2.go:322::handleWAFPutPermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetPermissionPolicy` | ✓ `simulators/aws/wafv2.go:323::handleWAFGetPermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeletePermissionPolicy` | ✓ `simulators/aws/wafv2.go:324::handleWAFDeletePermissionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GenerateMobileSdkReleaseUrl` | ✓ `simulators/aws/wafv2.go:326::handleWAFGenerateMobileSdkReleaseUrl` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetMobileSdkRelease` | ✓ `simulators/aws/wafv2.go:327::handleWAFGetMobileSdkRelease` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.ListMobileSdkReleases` | ✓ `simulators/aws/wafv2.go:328::handleWAFListMobileSdkReleases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.DeleteFirewallManagerRuleGroups` | ✓ `simulators/aws/wafv2.go:330::handleWAFDeleteFirewallManagerRuleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetRateBasedStatementManagedKeys` | ✓ `simulators/aws/wafv2.go:332::handleWAFGetRateBasedStatementManagedKeys` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action AWSWAF_20190729.GetTopPathStatisticsByTraffic` | ✓ `simulators/aws/wafv2.go:333::handleWAFGetTopPathStatisticsByTraffic` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Amplify associations are data-plane active: the associated WebACL's default
action and IP-set rules inspect actual hosted requests, terminal BLOCK actions
return HTTP 403, and enabled WebACL/rule visibility configurations retain the
request method, URI, headers, client address, action, response code, and
timestamp. `GetSampledRequests` filters that observed traffic by WebACL,
metric, scope, and the requested (at most three-hour) time window, reports the
real population size, and applies `MaxItems`. Official AWS SDK tests prove
blocking, sampling, disassociation, and the restored hosted response.
<!-- HAND-WRITTEN END -->
