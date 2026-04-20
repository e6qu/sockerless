# Live-AWS smoke tests

Idempotent shell scripts that drive sockerless against a real AWS account —
ECS + Lambda with real github.com / gitlab.com runners. Each script
corresponds to one runbook and cleans up on exit (success or failure),
so a broken run never leaves AWS residue.

Dispatched by `.github/workflows/aws-live.yml` (workflow_dispatch-only)
or run locally once credentials are exported.

## Prerequisites

- AWS credentials with ECS + Lambda + ECR + EFS + IAM + VPC + S3 +
  Cloud Map privileges in a scratch account.
- `terragrunt` + `terraform` on PATH.
- `docker` + `gh` on PATH (for the runner scripts).

## Entrypoints

| Script | Owns |
|---|---|
| `0-infra-up.sh` | terragrunt apply; caches outputs to `/tmp/ecs-out.json`. |
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
