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
| `PUT {topicScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridCreateEventSubscription` | ✓ | ✓ | n/a | Creates webhook subscription and delivers validation event. |
| `GET {topicScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridGetEventSubscription` | ✓ SDK read/list path | ✓ list path | n/a | Returns event subscription. |
| `GET {topicScope}/providers/Microsoft.EventGrid/eventSubscriptions` | ✓ `eventgrid.go::handleEventGridListEventSubscriptions` | ✓ `NewListByResourcePager` | ✓ `az rest` list | n/a | Lists topic event subscriptions. |
| `DELETE {topicScope}/providers/Microsoft.EventGrid/eventSubscriptions/{name}` | ✓ `eventgrid.go::handleEventGridDeleteEventSubscription` | ✓ cleanup via topic delete | ✓ cleanup via topic delete | n/a | Deletes event subscription. |
| `POST /api/events` on topic endpoint | ✓ `eventgrid.go::publishEventGridTopic` | ✓ direct returned endpoint | ✓ direct returned endpoint | n/a | Publishes Event Grid events and fans out to webhook subscriptions. |

## Open subtasks staged forward

- BUG-1211 / issue #247 tracks the broader Azure host-addressed data-plane DNS assumption for services that still return `*.localhost` data-plane hosts.
- BUG-1212 / issue #248 tracks local macOS execution for Azure Terraform provider tests.
- BUG-1214 / issue #250 tracks remaining Event Grid parity for domains, domain topics, system topics, partner topics where relevant, and their event subscriptions.

## Closed bugs

- BUG-1199 — foundational Azure Event Grid slice added with SDK, CLI, and Terraform coverage.
