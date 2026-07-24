# AWS Behavioral Patterns

This registry lists the asynchronous / background / dispatch patterns the AWS
simulator implements. Every persistent background evaluator, listener, or
fan-out dispatch path must be classified here, and each entry must point to a
behavioral test that proves the pattern end-to-end through a canonical SDK
client.

The `scripts/check-behavioral-coverage.sh` pre-commit check enforces that:

1. Every registry row uses an allowed classification and has existing source
   and test files.
2. Every `simulators/aws/sdk-tests/*behavioral*_test.go` file is registered.
3. Every newly-added persistent background loop or listener in
   `simulators/aws/*.go` is either registered or explicitly marked out-of-scope.

Allowed classifications:

- `background-evaluator` — periodic loop that re-evaluates cloud state and
  applies side effects (alarms, scaling, scheduling).
- `listener` — long-running network listener that clients reach directly
  (DNS server, runtime sidecar, proxy listener).
- `dispatch` — synchronous or asynchronous fan-out of an event to downstream
  targets (SNS fan-out, S3 notification, DLQ redrive, metric-filter on ingest).
- `audit` — cross-cutting behavioral assertions that do not map to a single
  background pattern (e.g., idempotency, error-code fidelity). Source may be a
  directory.

| Pattern | Classification | Source file | Behavioral test file |
| ------- | -------------- | ----------- | -------------------- |
| `cloudwatch-alarm-evaluator` | background-evaluator | `simulators/aws/cloudwatch_alarm_evaluator.go` | `simulators/aws/sdk-tests/behavioral_gate_test.go` |
| `application-autoscaling-evaluator` | background-evaluator | `simulators/aws/application_autoscaling_eval.go` | `simulators/aws/sdk-tests/behavioral_gate_test.go` |
| `eventbridge-scheduler-firing` | background-evaluator | `simulators/aws/scheduler_firing.go` | `simulators/aws/sdk-tests/scheduler_test.go` |
| `route53-dns-server` | listener | `simulators/aws/route53_dns.go` | `simulators/aws/sdk-tests/behavioral_gate_test.go` |
| `cloudwatch-logs-metric-filter` | dispatch | `simulators/aws/cloudwatch_logs_ops.go` | `simulators/aws/sdk-tests/behavioral_gate_test.go` |
| `sqs-dead-letter-redrive` | dispatch | `simulators/aws/sqs.go` | `simulators/aws/sdk-tests/behavioral_gate_test.go` |
| `sns-topic-fanout` | dispatch | `simulators/aws/sns.go` | `simulators/aws/sdk-tests/cloudwatch_alarm_sns_sqs_process_test.go` |
| `lambda-runtime-sidecar` | listener | `simulators/aws/lambda_runtime.go` | `simulators/aws/sdk-tests/lambda_test.go` |
| `behavioral-audit-misc` | audit | `simulators/aws` | `simulators/aws/sdk-tests/behavioral_audit_test.go` |
