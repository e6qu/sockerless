# Sockerless — Status

**85 phases (756 tasks). 661 bugs fixed, 0 open. Cloud build services for all 6 backends. Phase 86 no-AWS track complete.**

## Test Counts

| Category | Count |
|---|---|
| Core unit | 310 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 75 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 issues |

## ECS Live Testing

6 rounds against real AWS ECS Fargate (`eu-west-1`). Round 6: Docker CLI all pass, Podman pull+pods pass (container ops blocked by response format), Advanced 3/4. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Phase 86 — Complete Runner Support (no-AWS track)

Done: P86-001…007. Covers honest E2E matrix (no more silent `-wasm` variant fallback), per-hostname Cloud Map services + DNS search domains on ECS, best-effort Lambda `ContainerStop`/`Kill` with reverse-agent disconnect stub, lazy CloudWatch stream resolution for `docker logs -f`, agent-as-handler skeleton + overlay-image renderer for Lambda live exec, SaaS setup docs (`ECS_LIVE_SETUP.md`, `GITHUB_RUNNER_SAAS.md`, `GITLAB_RUNNER_SAAS.md`). Needs-AWS track (P86-008…013) is blocked on credentials + github.com / gitlab.com tokens. See `docs/runner-capability-matrix.md`.
