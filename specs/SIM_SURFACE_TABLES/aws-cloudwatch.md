# Sim surface — aws-cloudwatch

Surface registered in `simulators/aws/cloudwatch.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `Action Logs_20140328.CreateLogGroup` | ✓ `simulators/aws/cloudwatch.go:79::handleCWCreateLogGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeLogGroups` | ✓ `simulators/aws/cloudwatch.go:80::handleCWDescribeLogGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteLogGroup` | ✓ `simulators/aws/cloudwatch.go:81::handleCWDeleteLogGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.CreateLogStream` | ✓ `simulators/aws/cloudwatch.go:82::handleCWCreateLogStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeLogStreams` | ✓ `simulators/aws/cloudwatch.go:83::handleCWDescribeLogStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutLogEvents` | ✓ `simulators/aws/cloudwatch.go:84::handleCWPutLogEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetLogEvents` | ✓ `simulators/aws/cloudwatch.go:85::handleCWGetLogEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.FilterLogEvents` | ✓ `simulators/aws/cloudwatch.go:86::handleCWFilterLogEvents` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutRetentionPolicy` | ✓ `simulators/aws/cloudwatch.go:87::handleCWPutRetentionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.ListTagsForResource` | ✓ `simulators/aws/cloudwatch.go:88::handleCWListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.TagResource` | ✓ `simulators/aws/cloudwatch.go:89::handleCWTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.AssociateKmsKey` | ✓ `simulators/aws/cloudwatch.go:90::handleCWAssociateKmsKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DisassociateKmsKey` | ✓ `simulators/aws/cloudwatch.go:91::handleCWDisassociateKmsKey` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.EnableAlarmActions` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:173::handleCWJSONEnableAlarmActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DisableAlarmActions` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:174::handleCWJSONDisableAlarmActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.SetAlarmState` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:175::handleCWJSONSetAlarmState` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DescribeAlarmHistory` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:176::handleCWJSONDescribeAlarmHistory` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DescribeAlarmsForMetric` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:177::handleCWJSONDescribeAlarmsForMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutCompositeAlarm` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:178::handleCWJSONPutCompositeAlarm` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action EnableAlarmActions` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:594::handleCWQueryEnableAlarmActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DisableAlarmActions` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:595::handleCWQueryDisableAlarmActions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action SetAlarmState` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:596::handleCWQuerySetAlarmState` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAlarmHistory` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:597::handleCWQueryDescribeAlarmHistory` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAlarmsForMetric` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:598::handleCWQueryDescribeAlarmsForMetric` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutCompositeAlarm` | ✓ `simulators/aws/cloudwatch_alarm_ops.go:599::handleCWQueryPutCompositeAlarm` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutMetricAlarm` | ✓ `simulators/aws/cloudwatch_alarms.go:249::handleCWJSONPutMetricAlarm` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DescribeAlarms` | ✓ `simulators/aws/cloudwatch_alarms.go:250::handleCWJSONDescribeAlarms` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteAlarms` | ✓ `simulators/aws/cloudwatch_alarms.go:251::handleCWJSONDeleteAlarms` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutMetricAlarm` | ✓ `simulators/aws/cloudwatch_alarms.go:820::handleCWQueryPutMetricAlarm` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAlarms` | ✓ `simulators/aws/cloudwatch_alarms.go:821::handleCWQueryDescribeAlarms` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAlarms` | ✓ `simulators/aws/cloudwatch_alarms.go:822::handleCWQueryDeleteAlarms` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutAnomalyDetector` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:94::handleCWJSONPutAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DescribeAnomalyDetectors` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:95::handleCWJSONDescribeAnomalyDetectors` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteAnomalyDetector` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:96::handleCWJSONDeleteAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutInsightRule` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:97::handleCWJSONPutInsightRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DescribeInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:98::handleCWJSONDescribeInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.EnableInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:99::handleCWJSONEnableInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DisableInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:100::handleCWJSONDisableInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:101::handleCWJSONDeleteInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutAnomalyDetector` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:511::handleCWQueryPutAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeAnomalyDetectors` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:512::handleCWQueryDescribeAnomalyDetectors` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAnomalyDetector` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:513::handleCWQueryDeleteAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutInsightRule` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:514::handleCWQueryPutInsightRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DescribeInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:515::handleCWQueryDescribeInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action EnableInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:516::handleCWQueryEnableInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DisableInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:517::handleCWQueryDisableInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteInsightRules` | ✓ `simulators/aws/cloudwatch_anomaly_insight.go:518::handleCWQueryDeleteInsightRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutDashboard` | ✓ `simulators/aws/cloudwatch_dashboards.go:68::handleCWJSONPutDashboard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.GetDashboard` | ✓ `simulators/aws/cloudwatch_dashboards.go:69::handleCWJSONGetDashboard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.ListDashboards` | ✓ `simulators/aws/cloudwatch_dashboards.go:70::handleCWJSONListDashboards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteDashboards` | ✓ `simulators/aws/cloudwatch_dashboards.go:71::handleCWJSONDeleteDashboards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutDashboard` | ✓ `simulators/aws/cloudwatch_dashboards.go:242::handleCWQueryPutDashboard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetDashboard` | ✓ `simulators/aws/cloudwatch_dashboards.go:243::handleCWQueryGetDashboard` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListDashboards` | ✓ `simulators/aws/cloudwatch_dashboards.go:244::handleCWQueryListDashboards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteDashboards` | ✓ `simulators/aws/cloudwatch_dashboards.go:245::handleCWQueryDeleteDashboards` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.StartQuery` | ✓ `simulators/aws/cloudwatch_insights.go:35::handleCWStartQuery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetQueryResults` | ✓ `simulators/aws/cloudwatch_insights.go:36::handleCWGetQueryResults` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.StopQuery` | ✓ `simulators/aws/cloudwatch_insights.go:37::handleCWStopQuery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeQueries` | ✓ `simulators/aws/cloudwatch_insights.go:38::handleCWDescribeQueries` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutLogAlarm` | ✓ `simulators/aws/cloudwatch_log_alarms.go:67::handleCWJSONPutLogAlarm` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutAccountPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:147::handleCWPutAccountPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeAccountPolicies` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:148::handleCWDescribeAccountPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteAccountPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:149::handleCWDeleteAccountPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutQueryDefinition` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:152::handleCWPutQueryDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeQueryDefinitions` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:153::handleCWDescribeQueryDefinitions` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteQueryDefinition` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:154::handleCWDeleteQueryDefinition` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutResourcePolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:157::handleCWPutResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeResourcePolicies` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:158::handleCWDescribeResourcePolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteResourcePolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:159::handleCWDeleteResourcePolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDestination` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:162::handleCWPutDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeDestinations` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:163::handleCWDescribeDestinations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDestination` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:164::handleCWDeleteDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDestinationPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:165::handleCWPutDestinationPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.CreateDelivery` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:168::handleCWCreateDelivery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetDelivery` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:169::handleCWGetDelivery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDelivery` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:170::handleCWDeleteDelivery` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeDeliveries` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:171::handleCWDescribeDeliveries` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDeliverySource` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:174::handleCWPutDeliverySource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetDeliverySource` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:175::handleCWGetDeliverySource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeDeliverySources` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:176::handleCWDescribeDeliverySources` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDeliverySource` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:177::handleCWDeleteDeliverySource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDeliveryDestination` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:180::handleCWPutDeliveryDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetDeliveryDestination` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:181::handleCWGetDeliveryDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeDeliveryDestinations` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:182::handleCWDescribeDeliveryDestinations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDeliveryDestination` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:183::handleCWDeleteDeliveryDestination` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDeliveryDestinationPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:184::handleCWPutDeliveryDestinationPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetDeliveryDestinationPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:185::handleCWGetDeliveryDestinationPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDeliveryDestinationPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:186::handleCWDeleteDeliveryDestinationPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.CreateLogAnomalyDetector` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:189::handleCWCreateLogAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetLogAnomalyDetector` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:190::handleCWGetLogAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.ListLogAnomalyDetectors` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:191::handleCWListLogAnomalyDetectors` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteLogAnomalyDetector` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:192::handleCWDeleteLogAnomalyDetector` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutIndexPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:195::handleCWPutIndexPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteIndexPolicy` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:196::handleCWDeleteIndexPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeIndexPolicies` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:197::handleCWDescribeIndexPolicies` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeFieldIndexes` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:198::handleCWDescribeFieldIndexes` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeConfigurationTemplates` | ✓ `simulators/aws/cloudwatch_logs_extra2.go:201::handleCWDescribeConfigurationTemplates` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.StartLiveTail` | ✓ `simulators/aws/cloudwatch_logs_extra5.go:22::handleCWStartLiveTail` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetLogObject` | ✓ `simulators/aws/cloudwatch_logs_extra5.go:23::handleCWGetLogObject` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteLogStream` | ✓ `simulators/aws/cloudwatch_logs_ops.go:92::handleCWDeleteLogStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutMetricFilter` | ✓ `simulators/aws/cloudwatch_logs_ops.go:93::handleCWPutMetricFilter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeMetricFilters` | ✓ `simulators/aws/cloudwatch_logs_ops.go:94::handleCWDescribeMetricFilters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteMetricFilter` | ✓ `simulators/aws/cloudwatch_logs_ops.go:95::handleCWDeleteMetricFilter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.TestMetricFilter` | ✓ `simulators/aws/cloudwatch_logs_ops.go:96::handleCWTestMetricFilter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutSubscriptionFilter` | ✓ `simulators/aws/cloudwatch_logs_ops.go:97::handleCWPutSubscriptionFilter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeSubscriptionFilters` | ✓ `simulators/aws/cloudwatch_logs_ops.go:98::handleCWDescribeSubscriptionFilters` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteSubscriptionFilter` | ✓ `simulators/aws/cloudwatch_logs_ops.go:99::handleCWDeleteSubscriptionFilter` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteRetentionPolicy` | ✓ `simulators/aws/cloudwatch_logs_ops.go:100::handleCWDeleteRetentionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.TagLogGroup` | ✓ `simulators/aws/cloudwatch_logs_ops.go:101::handleCWTagLogGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.UntagLogGroup` | ✓ `simulators/aws/cloudwatch_logs_ops.go:102::handleCWUntagLogGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.ListTagsLogGroup` | ✓ `simulators/aws/cloudwatch_logs_ops.go:103::handleCWListTagsLogGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.UntagResource` | ✓ `simulators/aws/cloudwatch_logs_ops.go:104::handleCWUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.CreateExportTask` | ✓ `simulators/aws/cloudwatch_logs_ops.go:105::handleCWCreateExportTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DescribeExportTasks` | ✓ `simulators/aws/cloudwatch_logs_ops.go:106::handleCWDescribeExportTasks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.CancelExportTask` | ✓ `simulators/aws/cloudwatch_logs_ops.go:107::handleCWCancelExportTask` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutDataProtectionPolicy` | ✓ `simulators/aws/cloudwatch_logs_ops.go:108::handleCWPutDataProtectionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetDataProtectionPolicy` | ✓ `simulators/aws/cloudwatch_logs_ops.go:109::handleCWGetDataProtectionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteDataProtectionPolicy` | ✓ `simulators/aws/cloudwatch_logs_ops.go:110::handleCWDeleteDataProtectionPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutStorageTierPolicy` | ✓ `simulators/aws/cloudwatch_logs_syslog.go:36::handleCWPutStorageTierPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.GetStorageTierPolicy` | ✓ `simulators/aws/cloudwatch_logs_syslog.go:37::handleCWGetStorageTierPolicy` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.PutSyslogConfiguration` | ✓ `simulators/aws/cloudwatch_logs_syslog.go:38::handleCWPutSyslogConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.ListSyslogConfigurations` | ✓ `simulators/aws/cloudwatch_logs_syslog.go:39::handleCWListSyslogConfigurations` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action Logs_20140328.DeleteSyslogConfiguration` | ✓ `simulators/aws/cloudwatch_logs_syslog.go:40::handleCWDeleteSyslogConfiguration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:216::handleCWJSONPutMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.GetMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:217::handleCWJSONGetMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:218::handleCWJSONDeleteMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.ListMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:219::handleCWJSONListMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.StartMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:220::handleCWJSONStartMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.StopMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:221::handleCWJSONStopMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:492::handleCWQueryPutMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:493::handleCWQueryGetMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteMetricStream` | ✓ `simulators/aws/cloudwatch_metric_streams.go:494::handleCWQueryDeleteMetricStream` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:495::handleCWQueryListMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StartMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:496::handleCWQueryStartMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action StopMetricStreams` | ✓ `simulators/aws/cloudwatch_metric_streams.go:497::handleCWQueryStopMetricStreams` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutMetricData` | ✓ `simulators/aws/cloudwatch_metrics_json.go:39::handleCWJSONPutMetricData` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.GetMetricStatistics` | ✓ `simulators/aws/cloudwatch_metrics_json.go:40::handleCWJSONGetMetricStatistics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.ListMetrics` | ✓ `simulators/aws/cloudwatch_metrics_json.go:41::handleCWJSONListMetrics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutMetricData` | ✓ `simulators/aws/cloudwatch_metrics_query.go:27::handleCWQueryPutMetricData` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetMetricStatistics` | ✓ `simulators/aws/cloudwatch_metrics_query.go:28::handleCWQueryGetMetricStatistics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListMetrics` | ✓ `simulators/aws/cloudwatch_metrics_query.go:29::handleCWQueryListMetrics` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.GetMetricData` | ✓ `simulators/aws/cloudwatch_misc_ops.go:81::handleCWJSONGetMetricData` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.PutAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:82::handleCWJSONPutAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.GetAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:83::handleCWJSONGetAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.DeleteAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:84::handleCWJSONDeleteAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.ListAlarmMuteRules` | ✓ `simulators/aws/cloudwatch_misc_ops.go:85::handleCWJSONListAlarmMuteRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.TagResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:86::handleCWJSONTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.UntagResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:87::handleCWJSONUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GraniteServiceVersion20100801.ListTagsForResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:88::handleCWJSONListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetMetricData` | ✓ `simulators/aws/cloudwatch_misc_ops.go:527::handleCWQueryGetMetricData` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action PutAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:528::handleCWQueryPutAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action GetAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:529::handleCWQueryGetAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action DeleteAlarmMuteRule` | ✓ `simulators/aws/cloudwatch_misc_ops.go:530::handleCWQueryDeleteAlarmMuteRule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListAlarmMuteRules` | ✓ `simulators/aws/cloudwatch_misc_ops.go:531::handleCWQueryListAlarmMuteRules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action TagResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:532::handleCWQueryTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action UntagResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:533::handleCWQueryUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `Action ListTagsForResource` | ✓ `simulators/aws/cloudwatch_misc_ops.go:534::handleCWQueryListTagsForResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
