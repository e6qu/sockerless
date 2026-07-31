# Sim surface — azure-eventgrid

Surface registered in `simulators/azure/eventgrid_more.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `GET /providers/Microsoft.EventGrid/operations` | ✓ `simulators/azure/eventgrid_more.go:39::handleEventGridListOperations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /providers/Microsoft.EventGrid/topicTypes` | ✓ `simulators/azure/eventgrid_more.go:40::handleEventGridListTopicTypes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /providers/Microsoft.EventGrid/topicTypes/{topicTypeName}` | ✓ `simulators/azure/eventgrid_more.go:41::handleEventGridGetTopicType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /providers/Microsoft.EventGrid/topicTypes/{topicTypeName}/eventTypes` | ✓ `simulators/azure/eventgrid_more.go:42::handleEventGridListTopicTypeEventTypes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:76::handleEventGridListSubsGlobal` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventGrid/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:77::handleEventGridListSubsGlobal` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/locations/{location}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:78::handleEventGridListSubsRegional` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventGrid/locations/{location}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:79::handleEventGridListSubsRegional` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/topicTypes/{topicTypeName}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:80::handleEventGridListSubsForTopicType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventGrid/topicTypes/{topicTypeName}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:81::handleEventGridListSubsForTopicType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/locations/{location}/topicTypes/{topicTypeName}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:82::handleEventGridListSubsForTopicType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.EventGrid/locations/{location}/topicTypes/{topicTypeName}/eventSubscriptions` | ✓ `simulators/azure/eventgrid_more.go:83::handleEventGridListSubsForTopicType` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/partnerRegistrations` | ✓ `simulators/azure/eventgrid_partner.go:41::handleEventGridListPartnerRegistrationsBySub` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/partnerNamespaces` | ✓ `simulators/azure/eventgrid_partner.go:49::handleEventGridListPartnerNamespacesBySub` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/partnerConfigurations` | ✓ `simulators/azure/eventgrid_partner.go:67::handleEventGridListPartnerConfigurationsBySub` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /providers/Microsoft.EventGrid/verifiedPartners` | ✓ `simulators/azure/eventgrid_partner.go:72::handleEventGridListVerifiedPartners` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /providers/Microsoft.EventGrid/verifiedPartners/{verifiedPartnerName}` | ✓ `simulators/azure/eventgrid_partner.go:73::handleEventGridGetVerifiedPartner` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/topics` | ✓ `simulators/azure/eventgrid.go:65::handleEventGridListTopicsBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/domains` | ✓ `simulators/azure/eventgrid.go:78::handleEventGridListDomainsBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/systemTopics` | ✓ `simulators/azure/eventgrid.go:92::handleEventGridListSystemTopicsBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /subscriptions/{subscriptionId}/providers/Microsoft.EventGrid/partnerTopics` | ✓ `simulators/azure/eventgrid.go:105::handleEventGridListPartnerTopicsBySubscription` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /api/events` | ✓ `simulators/azure/eventgrid.go:127::handleEventGridPublishEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
