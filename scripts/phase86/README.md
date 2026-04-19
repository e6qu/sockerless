# Phase 86 live-AWS runbook scripts

Idempotent shell scripts for the Phase 86 live-AWS validation session.
Each script corresponds to a runbook in `_tasks/P86-AWS-manual-runbook.md`
and is designed to be dispatched from the `phase86-aws-live.yml`
workflow. Scripts clean up on exit (successful or failed) so a broken
run never leaves AWS residue.

## Prerequisites

- AWS credentials with ECS + Lambda + ECR + EFS + IAM + VPC + S3 +
  Cloud Map privileges in a scratch account (e.g. `729079515331`).
- `terragrunt` + `terraform` on PATH.
- `docker` + `gh` on PATH (for the matching GitHub / GitLab runbooks).

## Entrypoints

| Script | Runbook | Owns |
|---|---|---|
| `0-infra-up.sh` | Runbook 0 | terragrunt apply; caches outputs to `/tmp/ecs-out.json`. |
| `1-ecs-smoke.sh` | Runbook 1 | ECS basic smoke — docker run, ps, logs, exec, kill, cross-container DNS. |
| `2-lambda-baseline.sh` | Runbook 2 | Lambda basic — create, invoke, logs, update, delete. |
| `3-lambda-runtime-api.sh` | Runbook 3 | Lambda Runtime API + agent-as-handler — overlay build, exec. |
| `4-github-runner.sh` | Runbook 4 | github.com runner — PAT required via `GITHUB_PAT`. |
| `5-gitlab-runner.sh` | Runbook 5 | gitlab.com runner — runner token via `GITLAB_RUNNER_TOKEN`. |
| `6-teardown.sh` | Runbook 6 | terragrunt destroy + residue check. Always runs under `if: always()`. |

## Local dispatch

```sh
AWS_PROFILE=sockerless-live bash scripts/phase86/0-infra-up.sh
AWS_PROFILE=sockerless-live bash scripts/phase86/1-ecs-smoke.sh
# ...
AWS_PROFILE=sockerless-live bash scripts/phase86/6-teardown.sh
```

## GitHub Actions dispatch

The `.github/workflows/phase86-aws-live.yml` workflow is
`workflow_dispatch`-only — creds/tokens travel as encrypted inputs.
Teardown step runs with `if: always()` so a broken earlier step still
releases resources.

## Notes

- **Cost envelope:** ~$3 for a full Runbook 1-6 pass (NAT Gateway
  dominates; ~$0.05/hr).
- **Region:** `eu-west-1` is the default; change via `AWS_REGION`.
- **Residue:** `6-teardown.sh` refuses to exit 0 until `aws ecs
  list-clusters`, `aws lambda list-functions`, and `aws ec2
  describe-vpcs` all return empty for the sockerless prefix.
