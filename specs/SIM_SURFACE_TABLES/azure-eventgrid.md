# Sim surface — azure-eventgrid

Surface registered in `simulators/azure/eventgrid.go`. Rows below cover the Microsoft.EventGrid ARM control plane plus the custom-topic publish data plane implemented by the Azure simulator.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | cli-test | tf-test | notes |
|---|---|---|---|---|---|
| `PUT /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/topics/{topic}` | ✓ `eventgrid.go::handleEventGridCreateTopic` | ✓ `sdk-tests/eventgrid_test.go::TestEventGrid_TopicSubscriptionPublishSDK` | ✓ `cli-tests/eventgrid_test.go::TestEventGridCLI_TopicSubscriptionPublish` | ✓ `azurerm_eventgrid_topic` | Creates topic and returns a directly usable local publish endpoint for local simulator hosts. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/topics/{topic}` | ✓ `eventgrid.go::handleEventGridGetTopic` | ✓ Terraform read | ✓ via `az rest` read/list flow | ✓ Terraform read | Returns topic with provisioning state and endpoint. |
| `POST /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/topics/{topic}/listKeys` | ✓ `eventgrid.go::handleEventGridListTopicKeys` | ✓ `ListSharedAccessKeys` | ✓ `az rest` listKeys | ✓ `azurerm_eventgrid_topic` | Returns `key1` and `key2` in the real Event Grid topic access-key shape. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/topics` | ✓ `eventgrid.go::handleEventGridListTopicsByRG` | n/a | n/a | n/a | Resource-group topic list. |
| `GET /subscriptions/{sub}/providers/Microsoft.EventGrid/topics` | ✓ `eventgrid.go::handleEventGridListTopicsBySubscription` | n/a | n/a | n/a | Subscription topic list. |
| `DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/topics/{topic}` | ✓ `eventgrid.go::handleEventGridDeleteTopic` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes topic and topic-scoped subscriptions. |
| `PUT /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}` | ✓ `eventgrid.go::handleEventGridCreateDomain` | ✓ `TestEventGrid_DomainAndSystemTopicSDK` | ✓ `TestEventGridCLI_DomainAndSystemTopic` | ✓ `azurerm_eventgrid_domain` | Creates Event Grid domain resources and endpoint metadata. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}` | ✓ `eventgrid.go::handleEventGridGetDomain` | ✓ | ✓ | ✓ Terraform read | Returns domain metadata. |
| `POST /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}/listKeys` | ✓ `eventgrid.go::handleEventGridListDomainKeys` | ✓ `ListSharedAccessKeys` | ✓ `az rest` listKeys | ✓ `azurerm_eventgrid_domain` | Returns domain access keys. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains` | ✓ `eventgrid.go::handleEventGridListDomainsByRG` | ✓ pager | n/a | n/a | Resource-group domain list. |
| `GET /subscriptions/{sub}/providers/Microsoft.EventGrid/domains` | ✓ `eventgrid.go::handleEventGridListDomainsBySubscription` | n/a | n/a | n/a | Subscription domain list. |
| `DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}` | ✓ `eventgrid.go::handleEventGridDeleteDomain` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes domain, domain topics, and scoped subscriptions. |
| `PUT /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}/topics/{domainTopic}` | ✓ `eventgrid.go::handleEventGridCreateDomainTopic` | ✓ `DomainTopicsClient` | ✓ `az rest` | ✓ `azurerm_eventgrid_domain_topic` | Creates domain topic resources. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}/topics/{domainTopic}` | ✓ `eventgrid.go::handleEventGridGetDomainTopic` | ✓ | ✓ list path | ✓ Terraform read | Returns domain topic metadata. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}/topics` | ✓ `eventgrid.go::handleEventGridListDomainTopics` | ✓ pager | ✓ `az rest` list | n/a | Lists topics under a domain. |
| `DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/domains/{domain}/topics/{domainTopic}` | ✓ `eventgrid.go::handleEventGridDeleteDomainTopic` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes domain topic and scoped subscriptions. |
| `PUT /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/systemTopics/{systemTopic}` | ✓ `eventgrid.go::handleEventGridCreateSystemTopic` | ✓ `SystemTopicsClient` | ✓ `az rest` | ✓ `azurerm_eventgrid_system_topic` | Creates system topic resources and metric resource metadata. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/systemTopics/{systemTopic}` | ✓ `eventgrid.go::handleEventGridGetSystemTopic` | ✓ | ✓ list path | ✓ Terraform read | Returns system topic metadata. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/systemTopics` | ✓ `eventgrid.go::handleEventGridListSystemTopicsByRG` | ✓ pager | n/a | n/a | Resource-group system topic list. |
| `GET /subscriptions/{sub}/providers/Microsoft.EventGrid/systemTopics` | ✓ `eventgrid.go::handleEventGridListSystemTopicsBySubscription` | n/a | n/a | n/a | Subscription system topic list. |
| `DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/systemTopics/{systemTopic}` | ✓ `eventgrid.go::handleEventGridDeleteSystemTopic` | ✓ cleanup | ✓ cleanup | ✓ destroy | Deletes system topic and scoped subscriptions. |
| `PUT /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}` | ✓ `eventgrid.go::handleEventGridCreatePartnerTopic` | n/a current SDK module lacks partner-topic client | ✓ `TestEventGridCLI_PartnerTopicLifecycle` | n/a no `azurerm_eventgrid_partner_topic` resource | Creates partner topic resources through the public ARM route. |
| `PATCH /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}` | ✓ `eventgrid.go::handleEventGridUpdatePartnerTopic` | n/a | ✓ via same CLI surface route family | n/a | Updates partner topic tags/properties. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}` | ✓ `eventgrid.go::handleEventGridGetPartnerTopic` | n/a | ✓ | n/a | Returns partner topic metadata. |
| `POST /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}/activate` | ✓ `eventgrid.go::handleEventGridActivatePartnerTopic` | n/a | ✓ | n/a | Transitions partner topic activation/readiness state. |
| `POST /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}/deactivate` | ✓ `eventgrid.go::handleEventGridDeactivatePartnerTopic` | n/a | ✓ | n/a | Transitions partner topic activation/readiness state. |
| `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics` | ✓ `eventgrid.go::handleEventGridListPartnerTopicsByRG` | n/a | ✓ | n/a | Resource-group partner topic list. |
| `GET /subscriptions/{sub}/providers/Microsoft.EventGrid/partnerTopics` | ✓ `eventgrid.go::handleEventGridListPartnerTopicsBySubscription` | n/a | ✓ | n/a | Subscription partner topic list. |
| `DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.EventGrid/partnerTopics/{partnerTopic}` | ✓ `eventgrid.go::handleEventGridDeletePartnerTopic` | n/a | ✓ cleanup | n/a | Deletes partner topic and scoped subscriptions. |
| `PUT {eventGridScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridCreateEventSubscription` | ✓ topic/system-topic SDK clients where exposed | ✓ topic/system/partner/domain-topic `az rest` coverage | n/a | Creates webhook subscription and delivers validation event. |
| `GET {eventGridScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridGetEventSubscription` | ✓ SDK read/list path where exposed | ✓ list path | n/a | Returns event subscription. |
| `GET {eventGridScope}/providers/Microsoft.EventGrid/eventSubscriptions` | ✓ `eventgrid.go::handleEventGridListEventSubscriptions` | ✓ pagers where exposed | ✓ `az rest` list | n/a | Lists event subscriptions on topic, domain-topic, and partner-topic scopes. |
| `PUT {systemTopicScope}/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridCreateEventSubscription` | ✓ `SystemTopicEventSubscriptionsClient` | ✓ `az rest` | n/a | Creates system-topic event subscriptions on the public system-topic path. |
| `GET {systemTopicScope}/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridGetEventSubscription` | ✓ | ✓ list path | n/a | Returns system-topic event subscription. |
| `GET {systemTopicScope}/eventSubscriptions` | ✓ `eventgrid.go::handleEventGridListEventSubscriptions` | ✓ pager | ✓ `az rest` list | n/a | Lists system-topic event subscriptions. |
| `DELETE {eventGridScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` / `DELETE {systemTopicScope}/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridDeleteEventSubscription` | ✓ cleanup via scope delete | ✓ cleanup via scope delete | n/a | Deletes event subscriptions. |
| `POST /api/events` on topic endpoint | ✓ `eventgrid.go::publishEventGridTopic` | ✓ direct returned endpoint | ✓ direct returned endpoint | n/a | Publishes Event Grid events and fans out to webhook subscriptions. |

## Open subtasks staged forward

- BUG-1211 / issue #247 tracks the broader Azure host-addressed data-plane DNS assumption for services that still return `*.localhost` data-plane hosts.

## Closed bugs

- BUG-1199 — foundational Azure Event Grid slice added with SDK, CLI, and Terraform coverage.
- BUG-1214 / issue #250 — domains, domain topics, system topics, partner topics, and scoped event subscriptions added. Current SDK/provider exposure was checked before marking SDK/Terraform as not applicable for partner-topic direct resources.
