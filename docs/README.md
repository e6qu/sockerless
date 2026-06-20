# docs/

Topic guides, design notes, and research references. Specifications live in [`specs/`](../specs/); per-component documentation lives in each component's own `README.md` (see the [root README documentation index](../README.md#documentation)).

## CI runners & E2E testing

| Document | Description |
|----------|-------------|
| [`RUNNERS.md`](RUNNERS.md) | **Canonical CI runner wiring guide** — GitHub Actions + GitLab Runner against the cloud backends, token strategy, coverage matrix, runner hurdles catalog |
| [`GITHUB_RUNNER.md`](GITHUB_RUNNER.md) | GitHub Actions E2E tests using `act` against the Docker frontend |
| [`GITHUB_RUNNER_SAAS.md`](GITHUB_RUNNER_SAAS.md) | Running the official `actions/runner` against github.com with sockerless as the container runtime |
| [`GITLAB_RUNNER_DOCKER.md`](GITLAB_RUNNER_DOCKER.md) | GitLab Runner docker-executor E2E tests against a self-contained GitLab CE |
| [`GITLAB_RUNNER_SAAS.md`](GITLAB_RUNNER_SAAS.md) | Running `gitlab-runner` against gitlab.com with sockerless |
| [`runner-capability-matrix.md`](runner-capability-matrix.md) | What each backend can do when driving real CI runners through the Docker API |
| [`E2E_SMOKE_TESTS.md`](E2E_SMOKE_TESTS.md) | The three simulator-backed E2E smoke surfaces and which make targets drive them |
| [`ECS_LIVE_SETUP.md`](ECS_LIVE_SETUP.md) | Standing up the ECS backend against a real AWS account (prerequisite for the SAAS runner guides) |

## Build & dev infrastructure

| Document | Description |
|----------|-------------|
| [`MAKEFILE_STANDARD.md`](MAKEFILE_STANDARD.md) | **Authoritative build-system specification** — per-app target surface, shared recipes in [`make/`](../make/), fan-out rules |
| [`LOCAL_HTTPS_GATEWAY.md`](LOCAL_HTTPS_GATEWAY.md) | Optional Caddy HTTPS front door for the simulators (TLS, cloud-shaped hostnames) |
| [`OBSERVABILITY.md`](OBSERVABILITY.md) | Opt-in observability stack: OpenTelemetry Collector → VictoriaLogs + Jaeger |
| [`ADMIN_ORCHESTRATION.md`](ADMIN_ORCHESTRATION.md) | How `sockerless-admin` controls sims/backends/bleephubs declaratively from `sockerless.yaml` |
| [`BLEEPHUB_GH_CLI.md`](BLEEPHUB_GH_CLI.md) | Using the `gh` CLI against bleephub: hostname wiring, tokens, supported commands |

## Design notes

| Document | Description |
|----------|-------------|
| [`POD_MATERIALIZATION.md`](POD_MATERIALIZATION.md) | How a runner pod (long-lived runner + per-step sub-task containers) materializes on each of the 7 backends |
| [`ECS_SERVICES_DESIGN.md`](ECS_SERVICES_DESIGN.md) | Cross-container DNS for ECS via Cloud Map |
| [`ECS_EXPRESS_MODE.md`](ECS_EXPRESS_MODE.md) | AWS ECS Express Mode (Express Gateway services) — the managed Fargate + ALB + HTTPS + auto-scaling service and how the simulator composes its backing resources |
| [`LAMBDA_EXEC_DESIGN.md`](LAMBDA_EXEC_DESIGN.md) | `docker exec` on Lambda-backed containers via the mandatory reverse-agent model |

## Research & reference notes

| Document | Description |
|----------|-------------|
| [`VIBE_CODING.md`](VIBE_CODING.md) | Sourced anti-pattern catalog for AI-generated code and the policy/tooling response this repo adopts |
| [`GOLANG_STRONG_TYPING.md`](GOLANG_STRONG_TYPING.md) | Go type-strengthening research (status: research only) |
| [`terraform_min_timeouts_aws.md`](terraform_min_timeouts_aws.md) | Upstream terraform-provider-aws wait behavior affecting the AWS simulator terraform suite |
| [`terraform_min_timeouts_gcp.md`](terraform_min_timeouts_gcp.md) | Upstream terraform-provider-google wait behavior affecting the GCP simulator terraform suite |
| [`terraform_min_timeouts_azure.md`](terraform_min_timeouts_azure.md) | Upstream terraform-provider-azurerm wait behavior affecting the Azure simulator terraform suite |
