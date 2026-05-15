# simulator-aws

Local reimplementation of the AWS slice that sockerless touches. Not a mock — workloads execute through real Docker, ECS / Lambda tasks run with real exit semantics, ECR stores real image manifests, and the broader CDN / DNS / cert / WAF / Amplify / IAM surfaces respond on the real wire shapes that the AWS SDK v2 + AWS CLI + Terraform `aws` provider expect.

## Reference adaptor

The simulator exposes one HTTP endpoint (default `:4566`) that fronts all AWS services. Three external tools exercise that endpoint at AWS-API fidelity:

| Adaptor | Min version | What it proves |
|---|---|---|
| **AWS SDK for Go v2** (`github.com/aws/aws-sdk-go-v2/service/*`) | v1.30 | Wire-level SDK compatibility — request/response shapes, error envelopes, pagination, optimistic concurrency tokens. Covers 30+ services. |
| **`aws` CLI** | 2.17+ | Endpoint-override fidelity (`--endpoint-url`). CLI uses the same SDK but exercises a different argument-marshaling path. Some endpoints differ (e.g. Route 53 `/rrset/` with trailing slash). |
| **Terraform aws provider** | v6.32.1 | Full plan → apply → destroy round-trip across 60+ resource types (`aws_ecs_*`, `aws_lambda_*`, `aws_cloudfront_*`, `aws_route53_*`, `aws_wafv2_*`, `aws_amplify_*`, `aws_acm_*`, `aws_iam_*`, `aws_ecr_*`, `aws_s3_*`). Stresses cross-resource references and stateful drift detection. |

Anything any of these three tools does against the real AWS endpoint, it must do against this simulator. Gaps from that contract are real bugs (see [BUGS.md](../../BUGS.md)).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `sdk-tests/` — 30 packages (`ecs_test.go`, `ecr_test.go`, `cloudfront_test.go`, `route53_test.go`, `wafv2_test.go`, `amplify_test.go`, `acm_test.go`, `iam_slr_oidc_test.go`, …) | Real `aws-sdk-go-v2` clients against the sim. Per-op assertions on response shape + error codes. | 2026-05-15 (PR #159 P159.10) |
| `cli-tests/` — 30 packages (`ecs_test.go`, `iam_slr_oidc_test.go`, …) | Real `aws` CLI invoked via `os/exec`, parses CLI JSON output. | 2026-05-15 |
| `terraform-tests/` — `TestStackProductionShape` | Real Terraform `aws` v6.32.1 against the sim. Provisions CloudFront + ACM + WAFv2 + Route 53 ALIAS + Amplify + IAM SLR/OIDC + ECS + ECR + Cloud Map together, asserts cross-resource outputs (WAF.resource_arn == CloudFront.arn; Route 53 ALIAS target == CloudFront domain; ACM ARN region == us-east-1), then `terraform destroy`. | 2026-05-15 |
| `make simulators/aws/test` | Leaf-Makefile unit + integration suite per `docs/MAKEFILE_STANDARD.md`. | 2026-05-15 |

The SDK + Terraform tests are the load-bearing validation. CI runs all four on every PR (`.github/workflows/ci.yml`).

## Wiring the adaptor

```bash
# 1. Build + start the sim (default :4566).
cd simulators/aws
go build -tags noui -o simulator-aws .
SIM_LISTEN_ADDR=:4566 ./simulator-aws
```

```bash
# 2. Point any AWS client at it.
export AWS_ENDPOINT_URL=http://localhost:4566
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

aws ecs list-clusters
aws cloudfront list-distributions
aws iam create-service-linked-role --aws-service-name cloudfront.amazonaws.com
```

| Variable | Default | What it does |
|---|---|---|
| `SIM_LISTEN_ADDR` | `:4566` | Listen address (`host:port`). |
| `SIM_TLS_CERT`, `SIM_TLS_KEY` | unset | Enable HTTPS with the given cert/key. |
| `AWS_ENDPOINT_URL` | (client-side) | Tells the SDK / CLI / Terraform to route to the sim. |
| `AWS_DEFAULT_REGION` | `us-east-1` | The sim accepts any region; some validation (CloudFront → ACM us-east-1 pin) is region-aware. |

For Terraform:

```hcl
provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    ecs              = "http://localhost:4566"
    ecr              = "http://localhost:4566"
    cloudfront       = "http://localhost:4566"
    acm              = "http://localhost:4566"
    route53          = "http://localhost:4566"
    wafv2            = "http://localhost:4566"
    amplify          = "http://localhost:4566"
    iam              = "http://localhost:4566"
    # …any service you exercise.
  }
}
```

## Services

### AWS-JSON 1.1 (POST / + X-Amz-Target)

| Service | Target Prefix | Source file |
|---|---|---|
| **ECS** | `AmazonEC2ContainerServiceV20141113` | `ecs.go` |
| **ECR** | `AmazonEC2ContainerRegistry_V20150921` | `ecr.go` |
| **CloudWatch Logs** | `Logs_20140328` | `cloudwatch.go` |
| **Cloud Map** | `Route53AutoNaming_v20170314` | `cloudmap.go` |
| **ACM** | `CertificateManager` | `acm.go` |
| **WAFv2** | `AWSWAF_20190729` | `wafv2.go` |
| **KMS** | `TrentService` | `kms.go` |
| **Secrets Manager** | `secretsmanager` | `secretsmanager.go` |
| **DynamoDB** | `DynamoDB_20120810` | `dynamodb.go` |
| **SSM** | `AmazonSSM` | `ssm.go` |

### AWS Query Protocol (POST / + Action=)

| Service | Source file |
|---|---|
| **EC2** | `ec2.go` |
| **IAM** (roles, policies, instance profiles, service-linked roles, OIDC providers) | `iam.go` + `iam_slr_oidc.go` |
| **STS** | `sts.go` |

### REST APIs (path routing)

| Service | Base Path | Source file |
|---|---|---|
| **EFS** | `/2015-02-01/…` | `efs.go` |
| **Lambda** | `/2015-03-31/functions/…` | `lambda.go` |
| **S3** | `/{bucket}/{key}` | `s3.go` |
| **CloudFront** | `/2020-05-31/…` (REST + XML) | `cloudfront.go` + `cloudfront_policies.go` + `cloudfront_functions.go` + `cloudfront_keys.go` |
| **Route 53** | `/2013-04-01/…` (REST + XML) | `route53.go` |
| **Amplify** | `/apps/…` (REST + JSON, versionless) | `amplify.go` + `amplify_domains.go` |

Full per-verb wire shape: see [API_SPEC.md](API_SPEC.md).

## Sample — end-to-end production-shape stack

The `terraform-tests/TestStackProductionShape` exercise provisions a CloudFront-fronted application with WAF + ACM + Route 53 + Amplify + IAM SLR in a single `terraform apply`. Captured 2026-05-15 (sim port `:NNNN` shown as `:46241` here):

```bash
# Boot the sim
$ AWS_ENDPOINT_URL=http://127.0.0.1:46241 ./simulator-aws &

# Configure AWS SDK + Terraform
$ export AWS_ENDPOINT_URL=http://127.0.0.1:46241
$ export AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=us-east-1

# Apply the full stack
$ terraform apply -auto-approve
aws_cloudfront_distribution.tf_dist: Creation complete after 0.2s
  [id=E8E549340297351, domain_name=E8E549340297351.cloudfront.net]
aws_wafv2_web_acl_association.tf_assoc: Creation complete after 0.03s
aws_route53_record.tf_alias: Creation complete after 0.1s
aws_acm_certificate.tf_cert: Creation complete after 0.03s
  [arn=arn:aws:acm:us-east-1:000000000000:certificate/...]
aws_iam_service_linked_role.tf_slr_cloudfront: Creation complete after 0.02s
aws_iam_openid_connect_provider.tf_oidc: Creation complete after 0.02s
aws_amplify_app.tf_amplify: Creation complete after 0.01s

# Verify cross-resource references
$ terraform output -json | jq '. | with_entries(.value = .value.value)'
{
  "cloudfront_arn":             "arn:aws:cloudfront::000000000000:distribution/E8E549340297351",
  "cloudfront_domain_name":     "E8E549340297351.cloudfront.net",
  "wafv2_assoc_resource_arn":   "arn:aws:cloudfront::000000000000:distribution/E8E549340297351",
  "route53_alias_target_name":  "E8E549340297351.cloudfront.net",
  "acm_certificate_arn":        "arn:aws:acm:us-east-1:000000000000:certificate/...",
  "iam_slr_arn":                "arn:aws:iam::000000000000:role/aws-service-role/cloudfront.amazonaws.com/AWSServiceRoleForCloudFrontLogger_tftest"
}
```

The Go test asserts `wafv2_assoc_resource_arn == cloudfront_arn`, `route53_alias_target_name == cloudfront_domain_name`, and `acm_certificate_arn` starts with `arn:aws:acm:us-east-1:` — the three load-bearing cross-resource invariants of a CloudFront-fronted production stack.

```bash
$ terraform destroy -auto-approve
# Destroys all 30+ resources in dependency order.
```

## Building

```bash
cd simulators/aws
go build -tags noui -o simulator-aws .
```

## Testing

```bash
# SDK tests (AWS SDK v2 against the running sim — sim is built + booted per TestMain)
cd sdk-tests && go test -v ./...

# CLI tests (aws CLI shell-outs)
cd cli-tests && go test -v ./...

# Terraform tests (real terraform apply → assert outputs → destroy)
cd terraform-tests && go test -v ./...
```

Each test package's `TestMain` builds the simulator binary, finds a free port, boots the sim, waits for `/health`, runs the suite, then kills the sim. No external services needed.

## Known issues

None open for the services covered here. Closed within Phase 159:

- **BUG-991** — `docker run --rm` against `backends/docker` (used by these sim tests for workload execution) used to fail with `No such container`. Fixed by removing the Store-direct shortcut in `handleContainerWait`. See [BUGS.md](../../BUGS.md).
- **BUG-992** — `docker images` used to return empty even when the upstream daemon had images. Fixed by delegating to `s.self.ImageList`.

## What's out of scope

- **Edge propagation timing** — CloudFront distributions report `Status: Deployed` immediately; invalidations report `Completed` immediately. Real CloudFront cycles `InProgress → Deployed` over 5–15 minutes.
- **DNS resolution** — Route 53 stores records but does not serve them via UDP/53. The sim's purpose is API-shape parity, not actual DNS resolution. Use a separate dnsmasq sidecar if you need lookups.
- **WAF traffic inspection** — `GetSampledRequests` returns an empty list. The sim accepts WebACL rule definitions but doesn't actually filter traffic.
- **ACM cert auto-validation** — `RequestCertificate` with `ValidationMethod=DNS` stays `PENDING_VALIDATION` until you `ImportCertificate` to flip a cert to `ISSUED`. Real ACM polls Route 53 for the challenge CNAME.
- **Multi-region routing** — sim is single-region (defaults to `us-east-1`). Cross-region replication / failover is not modelled.
- **Cost / billing surfaces** — `cur`, `pricing`, `cost-explorer` are absent.
- **Real authentication** — sigv4 headers are accepted but not cryptographically verified.

See also: [API_SPEC.md](API_SPEC.md), [docs/POD_MATERIALIZATION.md](../../docs/POD_MATERIALIZATION.md), [specs/CLOUD_RESOURCE_MAPPING.md](../../specs/CLOUD_RESOURCE_MAPPING.md), [backends/ecs/README.md](../../backends/ecs/README.md), [backends/lambda/README.md](../../backends/lambda/README.md).
