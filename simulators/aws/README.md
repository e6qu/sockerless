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
├── sdk-tests/           SDK integration tests (35 tests)
├── cli-tests/           CLI integration tests (24 tests)
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

## Quick Start

All examples assume the simulator is running locally and the following environment is set:

```bash
# Build and start the simulator
cd simulators/aws && go build -o simulator-aws .
SIM_LISTEN_ADDR=:4566 ./simulator-aws

# In another terminal, configure credentials
export AWS_ENDPOINT_URL=http://localhost:4566
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1
```

### ECS

#### CLI

```bash
# Create a cluster
aws ecs create-cluster --cluster-name my-cluster
# => { "cluster": { "clusterName": "my-cluster", "status": "ACTIVE", ... } }

# Register a task definition with a command and CloudWatch log configuration
aws ecs register-task-definition \
  --family my-task \
  --requires-compatibilities FARGATE \
  --network-mode awsvpc \
  --cpu 256 --memory 512 \
  --container-definitions '[{
    "name": "app",
    "image": "alpine",
    "command": ["echo", "hello from ecs"],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/my-task",
        "awslogs-stream-prefix": "ecs"
      }
    }
  }]'
# => { "taskDefinition": { "family": "my-task", "revision": 1, ... } }

# Run the task (the command executes as a real process)
aws ecs run-task \
  --cluster my-cluster \
  --task-definition my-task \
  --count 1 \
  --launch-type FARGATE \
  --network-configuration 'awsvpcConfiguration={subnets=[subnet-12345]}'
# => { "tasks": [{ "taskArn": "arn:aws:ecs:...", "lastStatus": "RUNNING", ... }] }

# Wait a moment for the process to complete, then check task status
aws ecs describe-tasks --cluster my-cluster --tasks <TASK_ARN>
# => { "tasks": [{ "lastStatus": "STOPPED", "containers": [{ "exitCode": 0 }] }] }

# Read the CloudWatch logs produced by the task
aws logs filter-log-events --log-group-name /ecs/my-task
# => { "events": [{ "message": "hello from ecs", ... }] }
```

#### Go SDK

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func main() {
	ctx := context.Background()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	endpoint := "http://localhost:4566"

	ec := ecs.NewFromConfig(cfg, func(o *ecs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Create cluster
	ec.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String("my-cluster"),
	})

	// Register task definition with real execution
	tdOut, _ := ec.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("my-task"),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:    aws.String("app"),
			Image:   aws.String("alpine"),
			Command: []string{"echo", "hello from ecs"},
			LogConfiguration: &ecstypes.LogConfiguration{
				LogDriver: ecstypes.LogDriverAwslogs,
				Options: map[string]string{
					"awslogs-group":         "/ecs/my-task",
					"awslogs-stream-prefix": "ecs",
				},
			},
		}},
	})

	// Run task
	runOut, _ := ec.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String("my-cluster"),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets: []string{"subnet-12345"},
			},
		},
	})
	taskArn := runOut.Tasks[0].TaskArn

	// Wait for process to finish, then check status
	time.Sleep(2 * time.Second)
	desc, _ := ec.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String("my-cluster"),
		Tasks:   []string{*taskArn},
	})
	fmt.Println("Status:", *desc.Tasks[0].LastStatus) // "STOPPED"

	// Read logs
	cw := cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	logs, _ := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String("/ecs/my-task"),
	})
	for _, e := range logs.Events {
		fmt.Println(*e.Message)
	}
}
```

### Lambda

#### CLI

```bash
# Create an image-type function with a command
aws lambda create-function \
  --function-name my-func \
  --role arn:aws:iam::123456789012:role/test-role \
  --package-type Image \
  --code ImageUri=test:latest \
  --image-config '{"Command":["echo","hello from lambda"]}'
# => { "FunctionName": "my-func", "State": "Active", ... }

# Invoke the function (command runs as a real process)
aws lambda invoke --function-name my-func /dev/stdout
# => hello from lambda

# Check CloudWatch logs (auto-created under /aws/lambda/<name>)
aws logs filter-log-events --log-group-name /aws/lambda/my-func
# => { "events": [
#      { "message": "START RequestId: ..." },
#      { "message": "hello from lambda" },
#      { "message": "END RequestId: ..." },
#      { "message": "REPORT RequestId: ..." }
#    ] }
```

#### Go SDK

```go
package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func main() {
	ctx := context.Background()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	endpoint := "http://localhost:4566"

	lc := lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Create function with a command
	lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String("my-func"),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code:         &lambdatypes.FunctionCode{ImageUri: aws.String("test:latest")},
		ImageConfig:  &lambdatypes.ImageConfig{Command: []string{"echo", "hello from lambda"}},
	})

	// Invoke — the command runs as a real process
	out, _ := lc.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String("my-func"),
	})
	fmt.Println("Response:", string(out.Payload)) // "hello from lambda"

	// Read CloudWatch logs
	cw := cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	logs, _ := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String("/aws/lambda/my-func"),
	})
	for _, e := range logs.Events {
		fmt.Println(*e.Message)
	}
}
```

### CloudWatch Logs

#### CLI

```bash
# Create a log group
aws logs create-log-group --log-group-name /myapp/logs
# (no output on success)

# Create a log stream
aws logs create-log-stream --log-group-name /myapp/logs --log-stream-name stream-1

# Put log events (timestamps in milliseconds since epoch)
aws logs put-log-events \
  --log-group-name /myapp/logs \
  --log-stream-name stream-1 \
  --log-events \
    timestamp=$(date +%s000),message="request started" \
    timestamp=$(date +%s001),message="request completed"
# => { "nextSequenceToken": "..." }

# Get log events from a specific stream
aws logs get-log-events \
  --log-group-name /myapp/logs \
  --log-stream-name stream-1 \
  --start-from-head
# => { "events": [
#      { "message": "request started", "timestamp": ... },
#      { "message": "request completed", "timestamp": ... }
#    ] }

# Filter events across all streams in a group
aws logs filter-log-events \
  --log-group-name /myapp/logs \
  --filter-pattern "completed"
# => { "events": [{ "message": "request completed", ... }] }

# Describe log groups
aws logs describe-log-groups --log-group-name-prefix /myapp
# => { "logGroups": [{ "logGroupName": "/myapp/logs", ... }] }
```

#### Go SDK

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

func main() {
	ctx := context.Background()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}

	cw := cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String("http://localhost:4566")
	})

	// Create log group and stream
	cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String("/myapp/logs"),
	})
	cw.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String("/myapp/logs"),
		LogStreamName: aws.String("stream-1"),
	})

	// Put log events
	now := time.Now().UnixMilli()
	cw.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String("/myapp/logs"),
		LogStreamName: aws.String("stream-1"),
		LogEvents: []cwltypes.InputLogEvent{
			{Timestamp: aws.Int64(now), Message: aws.String("request started")},
			{Timestamp: aws.Int64(now + 1), Message: aws.String("request completed")},
		},
	})

	// Filter events across all streams
	out, _ := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:   aws.String("/myapp/logs"),
		FilterPattern:  aws.String("completed"),
	})
	for _, e := range out.Events {
		fmt.Println(*e.Message)
	}
}
```

### ECR

#### CLI

```bash
# Create a repository
aws ecr create-repository --repository-name my-app
# => { "repository": { "repositoryName": "my-app", "repositoryUri": "123456789012.dkr.ecr.us-east-1.localhost:4566/my-app", ... } }

# Describe repositories
aws ecr describe-repositories --repository-names my-app
# => { "repositories": [{ "repositoryName": "my-app", ... }] }

# Get an authorization token (base64-encoded "AWS:password")
aws ecr get-authorization-token
# => { "authorizationData": [{ "authorizationToken": "...", "proxyEndpoint": "..." }] }
```

#### Go SDK

```go
package main

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

func main() {
	ctx := context.Background()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}

	client := ecr.NewFromConfig(cfg, func(o *ecr.Options) {
		o.BaseEndpoint = aws.String("http://localhost:4566")
	})

	// Create repository
	repo, _ := client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String("my-app"),
	})
	fmt.Println("URI:", *repo.Repository.RepositoryUri)

	// Get authorization token
	auth, _ := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	token, _ := base64.StdEncoding.DecodeString(*auth.AuthorizationData[0].AuthorizationToken)
	fmt.Println("Credentials:", string(token)) // "AWS:<password>"
}
```

### S3

#### CLI

```bash
# Create a bucket
aws s3api create-bucket --bucket my-bucket --endpoint-url http://localhost:4566/s3
# (no output on success)

# Upload an object
aws s3api put-object \
  --bucket my-bucket \
  --key hello.txt \
  --body /dev/stdin --endpoint-url http://localhost:4566/s3 <<< "hello world"
# => { "ETag": "\"5eb63bbbe01eeed093cb22bb8f5acdc3\"" }

# Download an object
aws s3api get-object \
  --bucket my-bucket \
  --key hello.txt \
  --endpoint-url http://localhost:4566/s3 /dev/stdout
# => hello world

# List objects in a bucket
aws s3api list-objects-v2 --bucket my-bucket --endpoint-url http://localhost:4566/s3
# => { "Contents": [{ "Key": "hello.txt", "Size": 11, ... }] }
```

Note: S3 uses a `/s3/` path prefix. Pass `--endpoint-url http://localhost:4566/s3` (not the base URL) for CLI commands, or set `UsePathStyle: true` with the `/s3` suffix in the SDK.

#### Go SDK

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	ctx := context.Background()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://localhost:4566/s3")
		o.UsePathStyle = true
	})

	// Create bucket
	client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("my-bucket"),
	})

	// Put object
	client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("my-bucket"),
		Key:    aws.String("hello.txt"),
		Body:   bytes.NewReader([]byte("hello world")),
	})

	// Get object
	out, _ := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("my-bucket"),
		Key:    aws.String("hello.txt"),
	})
	defer out.Body.Close()
	data, _ := io.ReadAll(out.Body)
	fmt.Println(string(data)) // "hello world"
}
```
