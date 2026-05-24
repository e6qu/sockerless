# Sim surface — aws-wafv2

Surface registered in `simulators/aws/wafv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action AWSWAF_20190729.CreateWebACL` | ✓ `simulators/aws/wafv2.go:179::handleWAFCreateWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetWebACL` | ✓ `simulators/aws/wafv2.go:180::handleWAFGetWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.UpdateWebACL` | ✓ `simulators/aws/wafv2.go:181::handleWAFUpdateWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.DeleteWebACL` | ✓ `simulators/aws/wafv2.go:182::handleWAFDeleteWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListWebACLs` | ✓ `simulators/aws/wafv2.go:183::handleWAFListWebACLs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.AssociateWebACL` | ✓ `simulators/aws/wafv2.go:185::handleWAFAssociateWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.DisassociateWebACL` | ✓ `simulators/aws/wafv2.go:186::handleWAFDisassociateWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetWebACLForResource` | ✓ `simulators/aws/wafv2.go:187::handleWAFGetWebACLForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListResourcesForWebACL` | ✓ `simulators/aws/wafv2.go:188::handleWAFListResourcesForWebACL` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.CreateIPSet` | ✓ `simulators/aws/wafv2.go:190::handleWAFCreateIPSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetIPSet` | ✓ `simulators/aws/wafv2.go:191::handleWAFGetIPSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.UpdateIPSet` | ✓ `simulators/aws/wafv2.go:192::handleWAFUpdateIPSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.DeleteIPSet` | ✓ `simulators/aws/wafv2.go:193::handleWAFDeleteIPSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListIPSets` | ✓ `simulators/aws/wafv2.go:194::handleWAFListIPSets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.CreateRuleGroup` | ✓ `simulators/aws/wafv2.go:196::handleWAFCreateRuleGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetRuleGroup` | ✓ `simulators/aws/wafv2.go:197::handleWAFGetRuleGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.UpdateRuleGroup` | ✓ `simulators/aws/wafv2.go:198::handleWAFUpdateRuleGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.DeleteRuleGroup` | ✓ `simulators/aws/wafv2.go:199::handleWAFDeleteRuleGroup` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListRuleGroups` | ✓ `simulators/aws/wafv2.go:200::handleWAFListRuleGroups` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.CreateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:202::handleWAFCreateRegexSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetRegexPatternSet` | ✓ `simulators/aws/wafv2.go:203::handleWAFGetRegexSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.UpdateRegexPatternSet` | ✓ `simulators/aws/wafv2.go:204::handleWAFUpdateRegexSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.DeleteRegexPatternSet` | ✓ `simulators/aws/wafv2.go:205::handleWAFDeleteRegexSet` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListRegexPatternSets` | ✓ `simulators/aws/wafv2.go:206::handleWAFListRegexSets` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.TagResource` | ✓ `simulators/aws/wafv2.go:208::handleWAFTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.UntagResource` | ✓ `simulators/aws/wafv2.go:209::handleWAFUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.ListTagsForResource` | ✓ `simulators/aws/wafv2.go:210::handleWAFListTagsForResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `Action AWSWAF_20190729.GetSampledRequests` | ✓ `simulators/aws/wafv2.go:212::handleWAFGetSampledRequests` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
