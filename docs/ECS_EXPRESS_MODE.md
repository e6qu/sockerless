# Amazon ECS Express Mode

The Amazon Web Services (AWS) Elastic Container Service (ECS) simulator implements
**ECS Express Mode** (Express Gateway services) — the managed, Fargate-based ECS service
AWS launched on 2025-11-21. This document describes what the real service does, the
four-operation application programming interface (API) the simulator serves, how an
ECS Express Mode service compares to a hand-assembled vanilla ECS service, and how the
simulator composes the real underlying AWS resources to back it faithfully.

Cross-references: the per-cloud resource mapping lives in
[`specs/CLOUD_RESOURCE_MAPPING.md` § AWS ECS](../specs/CLOUD_RESOURCE_MAPPING.md#aws-ecs-backend-ecs);
the simulator's ECS operation table lives in
[`specs/SIM_SURFACE_TABLES/aws-ecs.md`](../specs/SIM_SURFACE_TABLES/aws-ecs.md).

## What ECS Express Mode is

ECS Express Mode is a managed deployment experience that turns a container image into a
production-ready, internet-reachable web service from three inputs:

1. **A container image**, plus its port and (optionally) command, environment, secrets, and logging.
2. **An execution role** (`executionRoleArn`, where Arn is an Amazon Resource Name (ARN)) —
   the task execution role ECS uses to pull the image and write logs.
3. **An infrastructure role** (`infrastructureRoleArn`, **required**) — the role ECS
   assumes to provision and manage the load balancer, certificate, security group, and
   auto-scaling resources on your behalf.

From those inputs ECS provisions, manages, and hands back as one unit:

- An **ECS Fargate service** running a managed task definition (single container named `Main`).
- An **Application Load Balancer (ALB)** (internet-facing for `PUBLIC`, internal for `PRIVATE`),
  with a Hypertext Transfer Protocol Secure (HTTPS) **listener** and a target group,
  consolidating **up to 25 ECS Express Mode services** behind a single ALB when their
  network configuration matches.
- A managed **AWS Certificate Manager (ACM) certificate** for Transport Layer Security (TLS)
  on the HTTPS listener.
- An **AWS-provided domain** — the ALB's Domain Name System (DNS) name — surfaced as the
  service's `ingressPaths[].endpoint` (`https://<alb-dns>`). No Amazon Route 53 setup is required.
- A security group (when you don't supply one) and **built-in target-tracking auto-scaling**
  (Application Auto Scaling on the Fargate service's desired count).

The result is that a single `CreateExpressGatewayService` call replaces the
`RegisterTaskDefinition` + `CreateService` + Elastic Load Balancing version 2 (ELBv2)
(load balancer + target group + listener + certificate) + security-group + Application Auto
Scaling choreography you would otherwise wire up yourself.

References: AWS ECS API Reference (`CreateExpressGatewayService`,
`DescribeExpressGatewayService`, `UpdateExpressGatewayService`,
`DeleteExpressGatewayService`) and the *Amazon ECS Developer Guide* "Express Mode" /
"Express Gateway services" section; launched 2025-11-21. Modeled in the simulator against
`aws-sdk-go-v2/service/ecs@v1.85.0` (which vendors the Express Gateway types) and
`terraform-provider-aws` v6.23.0+ (the `aws_ecs_express_gateway_service` resource).

## The API

All four operations are awsJson1.1 on the shared ECS service router, addressed by the
`X-Amz-Target` header `AmazonEC2ContainerServiceV20141113.<Op>`.

| Operation | Key request fields | Response shape |
|---|---|---|
| `CreateExpressGatewayService` | `infrastructureRoleArn` (**required**); `cluster`, `cpu`, `memory`, `executionRoleArn`, `taskRoleArn`, `healthCheckPath`, `serviceName`, `networkConfiguration`, `primaryContainer`, `scalingTarget`, `tags`, `taskDefinitionArn` (all optional) | `ECSExpressGatewayService` |
| `DescribeExpressGatewayService` | `serviceArn` (**required**); `include: [TAGS]` (optional) | `ECSExpressGatewayService` |
| `UpdateExpressGatewayService` | `serviceArn` (**required**); mutable config (`cpu`, `memory`, `executionRoleArn`, `taskRoleArn`, `healthCheckPath`, `networkConfiguration`, `primaryContainer`, `scalingTarget`, `taskDefinitionArn`) | `UpdatedExpressGatewayService` |
| `DeleteExpressGatewayService` | `serviceArn` (**required**) | `ECSExpressGatewayService` (status → `DRAINING`) |

### Request structure

- **`primaryContainer`** — `{ image, containerPort, command[], environment[{name,value}],
  secrets[{name,valueFrom}], repositoryCredentials{credentialsParameter},
  awsLogsConfiguration{logGroup,logStreamPrefix} }`.
- **`networkConfiguration`** — `{ securityGroups[], subnets[] }`.
- **`scalingTarget`** — `{ minTaskCount, maxTaskCount, autoScalingMetric, autoScalingTargetValue }`.
- **Mutual exclusion**: `taskDefinitionArn` cannot be combined with `primaryContainer`,
  `executionRoleArn`, `taskRoleArn`, `cpu`, or `memory` — those knobs derive the *managed*
  task definition, so supplying your own task definition replaces all of them. A supplied
  task definition must contain a container named `Main` with a single Transmission Control
  Protocol (TCP) port mapping and declare FARGATE compatibility.

### Response shapes

**`ECSExpressGatewayService`** (Create / Describe / Delete) carries:

- `serviceArn`, `serviceName`, `cluster`, `infrastructureRoleArn`, `createdAt`, `updatedAt`
- `status` — `{ statusCode, statusReason }`
- `activeConfigurations[]` — one `ExpressGatewayServiceConfiguration` per revision:
  `{ cpu, createdAt, executionRoleArn, healthCheckPath, ingressPaths[], memory,
  networkConfiguration, primaryContainer, scalingTarget, serviceRevisionArn,
  taskDefinitionArn, taskRoleArn }`
- `tags[]` (returned by Describe only when `include` contains `TAGS`)

**`UpdatedExpressGatewayService`** (Update) is a **different, narrower** shape — it returns
the single new revision rather than the full service: `cluster`, `createdAt`, `serviceArn`,
`serviceName`, `status`, `targetConfiguration` (the new
`ExpressGatewayServiceConfiguration`), and `updatedAt`. It does **not** carry
`infrastructureRoleArn`, `tags`, or `activeConfigurations`.

### Enums

| Enum | Field | Values |
|---|---|---|
| `AccessType` | `ingressPaths[].accessType` | `PUBLIC`, `PRIVATE` |
| `ExpressGatewayServiceScalingMetric` | `scalingTarget.autoScalingMetric` | `AVERAGE_CPU`, `AVERAGE_MEMORY`, `REQUEST_COUNT_PER_TARGET` |
| `ExpressGatewayServiceStatusCode` | `status.statusCode` | `ACTIVE`, `DRAINING`, `INACTIVE` |
| `ExpressGatewayServiceInclude` | `include` | `TAGS` |

### Defaults

| Field | Default |
|---|---|
| `cpu` | `256` |
| `memory` | `512` |
| `healthCheckPath` | `/ping` |
| `primaryContainer.containerPort` | `80` |
| `scalingTarget.autoScalingTargetValue` | `60` |
| `scalingTarget.autoScalingMetric` | `AVERAGE_CPU` (when a `scalingTarget` is supplied) |
| `cluster` | `default` |

### Errors

| Operation | Error codes |
|---|---|
| Create | `AccessDenied`, `Client`, `ClusterNotFound`, `InvalidParameter`, `PlatformTaskDefinitionIncompatibility`, `PlatformUnknown`, `Server` (500), `UnsupportedFeature` |
| Describe | the Create set **+** `ResourceNotFound` |
| Update / Delete | the Create set **+** `ServiceNotActive`, `ServiceNotFound` |

All errors return HTTP 400 except `Server` (500).

## ECS Express Mode vs vanilla ECS

| Aspect | ECS Express Mode | Vanilla ECS (assembled by hand) |
|---|---|---|
| **Inputs required** | Container image + execution role + infrastructure role; everything else defaulted | Task definition, service config, networking, load balancer, certificate, DNS, and auto-scaling — all authored explicitly |
| **Task definition** | **Managed** — ECS registers a single-container (`Main`) Fargate task def from `primaryContainer` + `cpu`/`memory`/roles | **User-authored** — you write and `RegisterTaskDefinition` the full container definitions |
| **Load balancer** | **Auto ALB** + HTTPS listener + target group, provisioned and managed for you; up to **25 services consolidated** behind one ALB | **User-managed** — create the ALB, target group, and listener via ELBv2 and wire them to the service yourself |
| **Domain / DNS** | **AWS-provided** — the ALB DNS name, surfaced as `ingressPaths[].endpoint`; no Route 53 work | **User-managed** — register a Route 53 record (or use the raw ALB DNS) yourself |
| **TLS** | **Managed ACM certificate** on the HTTPS listener | **User-managed** — request/import an ACM cert and attach it to the listener |
| **Auto-scaling** | **Built-in target-tracking** on desired count, driven by `autoScalingMetric`/`autoScalingTargetValue` | **User-configured** — register an Application Auto Scaling scalable target and scaling policy yourself |
| **Networking / security groups** | **Auto** — a security group is created when you don't supply one; PUBLIC ⇒ internet-facing, PRIVATE ⇒ internal | **User-managed** — author security groups and the `awsvpc` (Virtual Private Cloud, VPC) network configuration |
| **API surface** | One call: `CreateExpressGatewayService` | `RegisterTaskDefinition` + `CreateService` + ELBv2 (`CreateLoadBalancer`/`CreateTargetGroup`/`CreateListener`) + ACM + Elastic Compute Cloud (EC2) security groups + Application Auto Scaling (`RegisterScalableTarget`/`PutScalingPolicy`) |
| **Update model** | `UpdateExpressGatewayService` — mutable knobs produce a **new revision** and roll the backing service; returns `UpdatedExpressGatewayService{targetConfiguration}` | `UpdateService` (+ new task-def revision) and separate updates to the load balancer / scaling / DNS as needed |
| **Deletion** | `DeleteExpressGatewayService` — status → `DRAINING`, **cascade teardown** of all backing resources | **Manual teardown** — delete the service, then the listener, target group, load balancer, certificate, scaling policy, scalable target, and security group individually |
| **Use cases** | Production web services you want managed end-to-end with minimal config — HTTPS APIs, web frontends, internal services | Full control over every resource — bespoke routing, multi-target-group services, existing shared load balancers, custom scaling logic |

## How sockerless simulates it (cloud-slice assembly)

Per the cloud-slice principle, the simulator does **not** fake a monolithic "Express
service" object. It **composes the real underlying AWS resources** — each landing in the
same simulator store its own service API uses, so every one is independently describable
through that API. Implemented in `simulators/aws/ecs_express.go`:

| Backing resource | Simulator store | Describable via |
|---|---|---|
| ECS Fargate service (task def named `Main`, port mapping, `LoadBalancers` wired to the target group) | `ecsServices`, `ecsTaskDefinitions` | `DescribeServices`, `DescribeTaskDefinition` |
| ALB (internet-facing for PUBLIC / internal for PRIVATE) + target group (Internet Protocol (IP) targets) + HTTPS port 443 listener | `elbv2LoadBalancers`, `elbv2TargetGroups`, `elbv2Listeners` | ELBv2 `DescribeLoadBalancers` / `DescribeTargetGroups` / `DescribeListeners` |
| Managed ACM certificate for the HTTPS listener | `acmCertificates` | ACM `DescribeCertificate` |
| Security group (only when the caller supplies none) | `ec2SecurityGroups` | EC2 `DescribeSecurityGroups` |
| Application Auto Scaling scalable target + target-tracking policy | `appScalableTargets`, `appScalingPolicies` | Application Auto Scaling `DescribeScalableTargets` / `DescribeScalingPolicies` |

Key fidelity points the simulator preserves:

- **AWS-provided domain = the ALB DNS name.** The simulator mints
  `<name>-<hash>.elb.<region>.amazonaws.com` as the ALB `DNSName` and sets
  `ingressPaths[].endpoint = https://<albDNS>`.
- **25-per-ALB consolidation.** An Express service reuses an existing Express ALB whose
  network configuration (scheme + subnets + security groups) matches and that hosts fewer
  than 25 services; otherwise it creates a fresh ALB (and its ACM certificate). The shared
  ALB is reference-counted.
- **Scalable target + policy.** `resourceId = service/<cluster>/<svc>`,
  `scalableDimension = ecs:service:DesiredCount`, with the predefined metric mapped from
  `autoScalingMetric` (`AVERAGE_CPU` → `ECSServiceAverageCPUUtilization`, `AVERAGE_MEMORY`
  → `ECSServiceAverageMemoryUtilization`, `REQUEST_COUNT_PER_TARGET` →
  `ALBRequestCountPerTarget`).
- **Revisions on update.** `UpdateExpressGatewayService` derives a new
  `ExpressGatewayServiceConfiguration` revision from the current one, rolls the backing
  Fargate service to the (possibly new) task definition, and returns it as
  `targetConfiguration`.
- **DRAINING teardown cascade.** `DeleteExpressGatewayService` tears down the listener,
  target group, security group, scalable target, and policy; the shared ALB (and its
  certificate) is only removed when its last Express service is deleted; the backing
  Fargate service is set INACTIVE with desired count 0; the Express service then transitions
  to `DRAINING`.

The simulator is faithful to the real Express Gateway API — a client differs **only in
coordinates** (the endpoint URL and credentials), never in code paths. There is no
sockerless-aware or Express-specific shortcut: every backing resource is a real
simulator-side resource queryable through its own service slice.

## Usage

A client points at the simulator by setting only its endpoint coordinate; the request
shapes, identifiers, and operations are identical to those against real AWS.

### Software development kit (SDK) (`aws-sdk-go-v2`)

```go
cfg, _ := config.LoadDefaultConfig(ctx,
    config.WithRegion("us-east-1"),
    config.WithBaseEndpoint(simEndpoint), // the only sim-vs-cloud difference
)
client := ecs.NewFromConfig(cfg)

out, err := client.CreateExpressGatewayService(ctx, &ecs.CreateExpressGatewayServiceInput{
    InfrastructureRoleArn: aws.String("arn:aws:iam::123456789012:role/ecs-express-infra"),
    ExecutionRoleArn:      aws.String("arn:aws:iam::123456789012:role/ecs-execution"),
    PrimaryContainer: &types.ExpressGatewayContainer{
        Image:         aws.String("public.ecr.aws/nginx/nginx:latest"),
        ContainerPort: aws.Int32(80),
    },
})
// out.Service.ActiveConfigurations[0].IngressPaths[0].Endpoint == "https://<alb-dns>"
```

### Command-line interface (CLI) (`aws`)

```sh
aws ecs create-express-gateway-service \
  --endpoint-url "$SIM_ENDPOINT" \
  --infrastructure-role-arn arn:aws:iam::123456789012:role/ecs-express-infra \
  --execution-role-arn      arn:aws:iam::123456789012:role/ecs-execution \
  --primary-container '{"image":"public.ecr.aws/nginx/nginx:latest","containerPort":80}'

aws ecs describe-express-gateway-service --endpoint-url "$SIM_ENDPOINT" \
  --service-arn "$ARN" --include TAGS
aws ecs delete-express-gateway-service   --endpoint-url "$SIM_ENDPOINT" --service-arn "$ARN"
```

### Terraform (`aws_ecs_express_gateway_service`, provider v6.23.0+)

```hcl
provider "aws" {
  region                      = "us-east-1"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  endpoints {
    ecs = var.sim_endpoint # the only sim-vs-cloud difference
  }
}

resource "aws_ecs_express_gateway_service" "web" {
  name                    = "web"
  infrastructure_role_arn = aws_iam_role.express_infra.arn
  execution_role_arn      = aws_iam_role.execution.arn

  primary_container {
    image          = "public.ecr.aws/nginx/nginx:latest"
    container_port = 80
  }
}
```

## See also

- [`specs/CLOUD_RESOURCE_MAPPING.md` § AWS ECS](../specs/CLOUD_RESOURCE_MAPPING.md#aws-ecs-backend-ecs) — the authoritative per-cloud Docker-concept → AWS-resource mapping.
- [`specs/SIM_SURFACE_TABLES/aws-ecs.md`](../specs/SIM_SURFACE_TABLES/aws-ecs.md) — the simulator's ECS operation surface table.
- [`docs/ECS_SERVICES_DESIGN.md`](ECS_SERVICES_DESIGN.md) — cross-container DNS for ECS via Cloud Map.
- [`docs/ECS_LIVE_SETUP.md`](ECS_LIVE_SETUP.md) — standing up the ECS backend against a real AWS account.
