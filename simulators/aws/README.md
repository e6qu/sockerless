# simulator-aws

Local reimplementation of the AWS APIs used by the Sockerless ECS and Lambda backends. This is not a mock — ECS tasks run with real execution semantics (they stay running until the process exits or `StopTask` is called, just like real ECS), Lambda functions invoke and return responses, CloudWatch Logs are written and queryable, and ECR stores real image manifests.

## Services

### AWS JSON Protocol (X-Amz-Target header)

| Service | Target Prefix | Endpoints |
|---------|--------------|-----------|
| **ECS** | `AmazonEC2ContainerServiceV20141113` | CreateCluster, DescribeClusters, DeleteCluster, RegisterTaskDefinition, DeregisterTaskDefinition, DescribeTaskDefinition, RunTask, DescribeTasks, StopTask, ListTasks, ListTagsForResource |
| **ECR** | `AmazonEC2ContainerRegistry` | CreateRepository, DescribeRepositories, DeleteRepository, BatchGetImage, PutImage, BatchCheckLayerAvailability, GetAuthorizationToken, PutLifecyclePolicy, GetLifecyclePolicy, DeleteLifecyclePolicy, ListTagsForResource, TagResource |
| **CloudWatch Logs** | `Logs_20140328` | CreateLogGroup, DescribeLogGroups, DeleteLogGroup, PutRetentionPolicy, CreateLogStream, DescribeLogStreams, PutLogEvents, GetLogEvents, FilterLogEvents, ListTagsForResource, TagResource |
| **Cloud Map** | `Route53AutoNaming_v20170314` | CreatePrivateDnsNamespace, GetNamespace, DeleteNamespace, ListNamespaces, CreateService, GetService, ListServices, RegisterInstance, DeregisterInstance, ListInstances, DiscoverInstances, GetOperation |

### AWS Query Protocol (Action parameter)

| Service | Endpoints |
|---------|-----------|
| **EC2** | VPCs, Subnets, Internet Gateways, Elastic IPs, NAT Gateways, Route Tables, Security Groups, Network Interfaces |
| **IAM** | Roles (CRUD), Inline Policies, Managed Policy Attach/Detach, Instance Profiles |
| **STS** | GetCallerIdentity |

### REST APIs (path routing)

| Service | Base Path | Endpoints |
|---------|-----------|-----------|
| **EFS** | `/2015-02-01/` | File Systems, Mount Targets, Access Points, Lifecycle Policies, Tags |
| **Lambda** | `/2015-03-31/functions/` | CreateFunction, GetFunction, DeleteFunction, ListFunctions, UpdateFunctionConfiguration, Invoke |
| **S3** | `/s3/` | Buckets (list, create, delete, head), Objects (put, get, head, delete, list) |

## Building

```sh
cd simulators/aws
go build -o simulator-aws .
```

## Running

```sh
# Default port 4566
./simulator-aws

# Custom port
SIM_AWS_PORT=5000 ./simulator-aws

# With TLS
SIM_TLS_CERT=cert.pem SIM_TLS_KEY=key.pem ./simulator-aws
```

### SDK configuration

```sh
export AWS_ENDPOINT_URL=http://localhost:4566
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1
```

## Project structure

```
aws/
├── main.go              Entry point, service registration
├── ecs.go               ECS clusters, task definitions, tasks (890 lines)
├── ecr.go               ECR repositories, images, auth (392 lines)
├── ec2.go               VPCs, subnets, gateways, security groups (1,331 lines)
├── s3.go                Buckets and objects (358 lines)
├── iam.go               Roles, policies (289 lines)
├── sts.go               GetCallerIdentity (24 lines)
├── cloudwatch.go        Log groups, streams, events (485 lines)
├── cloudmap.go          Namespaces, services, instances (525 lines)
├── efs.go               File systems, mount targets, access points (461 lines)
├── lambda.go            Functions, invoke (354 lines)
├── shared/              Shared simulator framework
├── sdk-tests/           SDK integration tests (17 tests)
├── cli-tests/           CLI integration tests (21 tests)
└── terraform-tests/     Terraform apply/destroy tests
```

## Guides

- [Using with the AWS CLI](docs/cli.md)
- [Using with Terraform](docs/terraform.md)
- [Using with boto3 (Python)](docs/python-sdk.md)

## Testing

```sh
# SDK tests (uses AWS SDK v2)
cd sdk-tests && go test -v ./...

# CLI tests (uses aws CLI)
cd cli-tests && go test -v ./...

# Terraform tests
cd terraform-tests && go test -v ./...
```

Tests build the simulator binary, start it on a free port, run the test suite, and shut it down. No external dependencies needed.

## Execution model

ECS tasks have no native execution timeout — they run until the container process exits or `StopTask` is called. The simulator faithfully reproduces this: started tasks remain in `RUNNING` state until explicitly stopped. When a task definition includes an entrypoint/command, the simulator executes it as a real process and streams output to CloudWatch logs. Lambda invocations are synchronous and return immediately with the function response.
