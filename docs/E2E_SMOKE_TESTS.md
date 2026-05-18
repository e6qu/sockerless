# E2E smoke tests

This repo has three simulator-backed E2E smoke surfaces. All of them do real
work against local simulators or local Docker-in-Docker harnesses; none use
mocks or synthetic cloud state. Simulators should behave like the cloud API
surface with only endpoint routing changed; SDKs, CLIs, Terraform providers,
and backend registry clients should keep cloud-shaped resource names and image
refs.

## FaaS backend Go smokes

These run inside the backend Go integration harnesses and exercise the runner
lifecycle shape against local simulators:

`ContainerCreate -> ContainerStart -> ContainerExec -> ContainerExec -> ContainerWait -> ContainerRemove`

Use these when changing reverse-agent bootstrap, service/app keep-alive, wait,
remove, or FaaS backend lifecycle code.

```bash
make faas-smoke-test-lambda
make faas-smoke-test-cloudrun
make faas-smoke-test-gcf
make faas-smoke-test-aca
make faas-smoke-test-azf

make faas-smoke-test-aws
make faas-smoke-test-gcp
make faas-smoke-test-azure
make faas-smoke-test-all
```

Path-delegated equivalents:

```bash
make backends/lambda/test-faas-smoke
make backends/cloudrun/test-faas-smoke
make backends/cloudrun-functions/test-faas-smoke
make backends/aca/test-faas-smoke
make backends/azure-functions/test-faas-smoke
```

CI runs `make faas-smoke-test-all` in the main `test` job after the normal
backend package suites.

## GitHub runner smokes

These build per-cloud Docker images that run the official GitHub Actions runner
against sockerless backends pointed at simulator endpoints.

```bash
make e2e-github-ecs
make e2e-github-lambda
make e2e-github-cloudrun
make e2e-github-gcf
make e2e-github-aca
make e2e-github-azf
make e2e-github-all
```

The older `smoke-test-act-*` targets run the act-based smoke harness:

```bash
make smoke-test-act-ecs
make smoke-test-act-cloudrun
make smoke-test-act-aca
make smoke-test-act-all
```

## GitLab runner smokes

These run GitLab Runner or gitlab-ci-local style Docker executor flows against
simulator-endpoint backends.

```bash
make e2e-gitlab-ecs
make e2e-gitlab-lambda
make e2e-gitlab-cloudrun
make e2e-gitlab-gcf
make e2e-gitlab-aca
make e2e-gitlab-azf
make e2e-gitlab-all
```

Docker Compose smoke targets:

```bash
make smoke-test-gitlab-ecs
make smoke-test-gitlab-cloudrun
make smoke-test-gitlab-aca
make smoke-test-gitlab-all
```

Upstream tool suites:

```bash
make upstream-test-act-{ecs,lambda,cloudrun,gcf,aca,azf,all}
make upstream-test-gcl-{ecs,lambda,cloudrun,gcf,aca,azf,all}
```

## Go cross-backend E2E

The `tests/` module drives real Docker API calls through sockerless. It can run
against the default local backend setup or against simulator-backed endpoints
configured by the harness.

```bash
make tests/test
make tests/test-integration
```

Direct form:

```bash
cd tests
go test -v ./...
SOCKERLESS_SIM=ecs,cloudrun,aca go test -v -timeout 15m ./...
```

## CI placement

The PR CI currently exercises:

- Backend package integration suites in `.github/workflows/ci.yml`.
- FaaS runner lifecycle smokes through `make faas-smoke-test-all`.
- Cross-backend Go E2E through `cd tests && go test -v -timeout 5m ./...`.
- Docker smoke images for ECS, Cloud Run, and ACA in the `smoke` job.
- Simulator SDK/CLI suites in the `sim` matrix.

Live-cloud validation uses the same Make entry points where possible and
requires operator-provisioned cloud projects/subscriptions plus authenticated
cloud CLIs.
