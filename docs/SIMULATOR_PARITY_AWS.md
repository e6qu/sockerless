# AWS Simulator Parity

Which AWS service slices `simulators/aws/` covers, for which surfaces,
and where the runner path relies on each one.

**Status legend:**

- ✔ **implemented** — simulator covers what sockerless uses, with SDK + CLI + terraform tests (or documented exemption).
- ◐ **partial** — some endpoints missing; not on sockerless's runner path.
- ✖ **not implemented** — sockerless uses this but simulator doesn't cover it. Tracked in BUGS.md.
- N/A — sockerless doesn't use this slice.

Current bug count: 707 total / 707 fixed / 0 open on the AWS side of Phase 86.

## Runner-path slices

| Slice | Status | File(s) | Runner usage |
|---|---|---|---|
| **ECS (control plane)** — RegisterTaskDefinition, RunTask, DescribeTasks, StopTask, ListTasks, UpdateTask, DescribeTaskDefinition, DeregisterTaskDefinition, CreateCluster / DeleteCluster, ListClusters, ListContainerInstances | ✔ | `ecs.go` | ECS backend runs every sockerless-managed container as a Fargate task. |
| **ECS Exec** — ExecuteCommand + SSM session-channel tunnel | ✔ | `ecs.go` + `ssm.go` (session channel) | `docker exec` against ECS backend. |
| **Fargate launch-type** — PlatformVersion, NetworkConfiguration, awsvpcConfiguration | ✔ | `ecs.go` | Every RunTask is Fargate. |
| **ECR** — CreateRepository, DescribeRepositories, DeleteRepository, GetAuthorizationToken, BatchGetImage, PutImage, BatchDeleteImage, BatchCheckLayerAvailability, PutLifecyclePolicy / GetLifecyclePolicy / DeleteLifecyclePolicy, ListTagsForResource, TagResource | ✔ | `ecr.go` | ECS + Lambda backends push overlay images here. |
| **ECR pull-through cache** — CreatePullThroughCacheRule, DescribePullThroughCacheRules, DeletePullThroughCacheRule (BUG-696) | ✔ | `ecr.go` | ECS backend rewrites Docker Hub refs via pull-through prefix. |
| **OCI Distribution v2** — `/v2/`, manifests, blobs, uploads (push + pull) | ✔ | `ecr.go` | ECR registry endpoint uses the OCI spec. |
| **Lambda (control plane)** — CreateFunction, GetFunction, ListFunctions, DeleteFunction, UpdateFunctionConfiguration, Invoke, CreateFunctionUrlConfig | ✔ | `lambda.go` | Lambda backend. |
| **Lambda Runtime API** (BUG-705) — `/2018-06-01/runtime/invocation/next`, `/response`, `/error`, `/runtime/init/error` — per-invocation sidecar | ✔ | `lambda_runtime.go` | Agent-as-handler containers use this to fetch payload + post result. |
| **CloudWatch Logs** — CreateLogGroup, CreateLogStream, PutLogEvents, FilterLogEvents, GetLogEvents, DescribeLogStreams | ✔ | `cloudwatch.go` | `docker logs` against ECS / Lambda reads here. |
| **Cloud Map / ServiceDiscovery** (BUG-701) — CreatePrivateDnsNamespace, CreateService, RegisterInstance, DeregisterInstance, DeleteNamespace, ListServices, ListInstances; namespace backed by a real Docker network | ✔ | `cloudmap.go` | Cross-task DNS: service → IP resolution via `<svc>.<ns>.local`. |
| **EC2** — DescribeSubnets, DescribeVpcs, CreateSecurityGroup, AuthorizeSecurityGroupIngress, DeleteSecurityGroup, DescribeSecurityGroups; pre-registers `vpc-sim` + `subnet-sim` on startup (BUG-699) | ✔ | `ec2.go` | ECS awsvpcConfiguration + per-network security group. |
| **EFS** — CreateFileSystem, DescribeFileSystems, DeleteFileSystem, CreateMountTarget, DescribeMountTargets, DeleteMountTarget | ✔ | `efs.go` | Docker volumes back to EFS for ECS tasks. |
| **STS** — GetCallerIdentity, AssumeRole | ✔ | `sts.go` | Auth bootstrap. |
| **IAM** — GetRole, ListRoles, CreateRole (minimal) | ✔ | `iam.go` | Execution / task role resolution. |
| **S3** — minimal slice used for task-def hashing + terraform state — CreateBucket, PutObject, GetObject, DeleteObject, ListBuckets, HeadBucket | ✔ | `s3.go` | Terraform `aws_s3_bucket` + artifact storage. |
| **CloudWatch Metrics** — PutMetricData (accepted, not visualized) | ◐ | `cloudwatch_metrics.go` | Backends push metrics; not read on runner path. |

## Out-of-scope (N/A) slices

Sockerless doesn't touch these, so the simulator doesn't implement them:

- AWS Batch, Lambda Aliases/Versions beyond default, CloudFront, RDS, DynamoDB, SQS, SNS, Kinesis, EventBridge, CodeBuild/Deploy/Pipeline, Route53 (using Cloud Map instead), KMS, Secrets Manager (Lambda backend uses env vars).

## Exit check

No ✖ rows. All runner-path slices are implemented with SDK + CLI + terraform tests (or documented exemption via `simulators/aws/tests-exempt.txt`).
