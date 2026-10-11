# Live-AWS smoke tests

Idempotent shell scripts that drive sockerless against a real AWS account —
ECS + Lambda with real github.com / gitlab.com runners. Each script
corresponds to one runbook and cleans up on exit (success or failure),
so a broken run never leaves AWS residue.

Dispatched by `.github/workflows/live-tests-ecs.yml` (ECS + runner
jobs) and `.github/workflows/live-tests-lambda.yml` (Lambda jobs) —
both workflow_dispatch-only — or run locally once credentials are
exported.

## Prerequisites

- AWS credentials with ECS + Lambda + ECR + EFS + IAM + VPC + S3 +
  Cloud Map privileges in a scratch account.
- `terragrunt` + `terraform` on PATH.
- `docker` + `gh` on PATH (for the runner scripts).
- Amazon ECS scripts: `SOCKERLESS_ECS_CPU_ARCHITECTURE` (`X86_64` or `ARM64`).
- AWS Lambda scripts: `SOCKERLESS_LAMBDA_ARCHITECTURE` (as the Lambda module was applied with), `SOCKERLESS_AGENT_BINARY` and `SOCKERLESS_LAMBDA_BOOTSTRAP` built for it, and `SOCKERLESS_CALLBACK_URL` — a `wss://…/v1/lambda/reverse` address the functions reach and that forwards to the backend's port 2376. Every function's bootstrap overlay is built in the Lambda module's AWS CodeBuild project.

## Entrypoints

| Script | Owns |
|---|---|
| `lib.sh` | Sourced by the others: the backends' environment from the cached Terraform outputs (the same mapping `tests/terraform-integration/run-test.sh` applies against the simulator), and a backend started and waited on. |
| `0-infra-up.sh` | terragrunt apply in `TG_DIR`; caches outputs to `INFRA_OUTPUT` (the ECS scripts read `/tmp/ecs-out.json`, the Lambda scripts `/tmp/ecs-infra-out.json` and `/tmp/aws-infra-out.json`). |
| `1-ecs-smoke.sh` | ECS basic smoke — docker run, ps, logs, exec, kill, cross-container DNS. |
| `2-lambda-baseline.sh` | Lambda basic — create, invoke, logs, update, delete. |
| `3-lambda-runtime-api.sh` | Lambda Runtime API + agent-as-handler — overlay build, exec. |
| `4-github-runner.sh` | github.com runner — PAT required via `GITHUB_PAT`. |
| `5-gitlab-runner.sh` | gitlab.com runner — runner token via `GITLAB_RUNNER_TOKEN`. |
| `6-teardown.sh` | terragrunt destroy + residue check. Always runs under `if: always()`. |

## Local dispatch

```sh
AWS_PROFILE=sockerless-live bash smoke-tests/aws-live/0-infra-up.sh
AWS_PROFILE=sockerless-live bash smoke-tests/aws-live/1-ecs-smoke.sh
# ...
AWS_PROFILE=sockerless-live bash smoke-tests/aws-live/6-teardown.sh
```

## Notes

- **Cost envelope:** ~$3 for a full end-to-end pass (NAT Gateway
  dominates; ~$0.05/hr).
- **Region:** `eu-west-1` default; override via `AWS_REGION`.
- **Residue:** `6-teardown.sh` exits non-zero if `aws ecs list-clusters`,
  `aws lambda list-functions`, or Cloud Map namespaces still contain
  sockerless resources.
