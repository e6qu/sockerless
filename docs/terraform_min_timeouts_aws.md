# Terraform AWS Provider Minimum Waits

This note records upstream wait behavior that affects the AWS simulator Terraform
suite. It is not simulator behavior. These waits come from
`hashicorp/terraform-provider-aws` v6.47.0, which is the version pinned by the
AWS Terraform harness.

Terraform resource `timeouts {}` blocks cap an operation's total deadline. They
do not tune the provider's internal `Delay`, `MinTimeout`, `PollInterval`, or
consecutive-observation requirements. AWS provider `max_retries` / `retry_mode`
controls AWS SDK API retries, not Terraform resource waiters.

## Waits Relevant To Current Coverage

| Surface | Provider source | Hard wait behavior |
|---|---|---|
| RDS DB instance create/delete | [`internal/service/rds/instance.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/rds/instance.go#L2896-L2900) | `Delay: 1m`, `PollInterval: 10s`, `ContinuousTargetOccurence: 3` for available/deleted waits. |
| RDS DB snapshot create | [`internal/service/rds/snapshot.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/rds/snapshot.go#L344-L351) | `Delay: 30s`, `MinTimeout: 10s`. |
| ElastiCache cluster create/delete | [`create`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/elasticache/cluster.go#L879-L891), [`delete`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/elasticache/cluster.go#L903-L918) | `Delay: 30s`, `MinTimeout: 10s`. |
| SQS queue attributes/delete | [`internal/service/sqs/queue.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/sqs/queue.go#L607-L630) | Attributes require 6 consecutive matching observations with `MinTimeout: 5s`; delete requires 15 consecutive missing observations with `MinTimeout: 3s`. |
| SQS missing queue error | [`consts.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/sqs/consts.go#L44), [`queue.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/sqs/queue.go#L367-L375) | The provider keys on `AWS.SimpleQueueService.NonExistentQueue`; simulators must return that AWS Query error code/header shape. |
| S3 bucket lifecycle configuration | [`internal/service/s3/bucket_lifecycle_configuration.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/s3/bucket_lifecycle_configuration.go#L694-L702) | `Delay: 10s`, `PollInterval: 5s`, `ContinuousTargetOccurence: 10`. |
| CloudFront distribution deployed/deleted | [`internal/service/cloudfront/distribution.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/cloudfront/distribution.go#L1437-L1444) | `Delay: 30s`, `MinTimeout: 15s` for deployed waits. |
| Route 53 change INSYNC | [`internal/service/route53/change_info.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/route53/change_info.go#L59-L72) | Uses provider-selected `Delay`, `MinTimeout`, and `PollInterval` values for change propagation. |
| ELBv2 load balancer active/deleted | [`internal/service/elbv2/load_balancer.go`](https://github.com/hashicorp/terraform-provider-aws/blob/v6.47.0/internal/service/elbv2/load_balancer.go#L1194-L1201) | `Delay: 30s`, `MinTimeout: 10s`. |

## Test Implication

The AWS Terraform harness must keep real terraform-provider-aws apply/read/destroy
coverage, but high-wait surfaces should be split into focused Go test packages
when the accumulated provider waits would exceed the repository's 5-minute
per-test cap. Raising the timeout or mocking provider behavior would hide the
actual public Terraform client contract.
