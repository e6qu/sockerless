# Sockerless — Current Status

**Phase 90 + unified image management complete. 756 tasks done across 85 phases. 583 bugs fixed (45 sprints). 0 open bugs.**

## Test Results

| Category | Count |
|---|---|
| Core unit tests | 294 PASS (`cd backends/core && go test -v ./...`) |
| Frontend tests | 7 PASS (TLS + mux) |
| UI tests (Vitest) | 92 PASS (13 packages) |
| Admin tests | 87 PASS |
| Admin Playwright E2E | 17 PASS |
| SPAHandler tests | 5 PASS |
| bleephub | 304 unit + 9 integration + 1 gh CLI |
| Shared ProcessRunner | 15 PASS |

### E2E & Integration

| Suite | Count | Command |
|---|---|---|
| Sim-backend integration | 75 PASS | `make sim-test-all` |
| GitHub E2E | 186 PASS (31 × 6 backends) | `make e2e-github-{backend}` |
| GitLab E2E | 132 PASS (22 × 6 backends) | `make e2e-gitlab-{backend}` |
| Upstream gitlab-ci-local | 216 PASS (36 × 6 backends) | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS | `make tf-int-test-all` |
| Cloud SDK tests | AWS 42, GCP 43, Azure 38 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 26, GCP 21, Azure 19 | `make docker-test` per cloud |
| Lint (18 modules) | 0 issues | `make lint` |

## Architecture

### Backends (7)

| Backend | Cloud | Execution | Agent |
|---|---|---|---|
| docker | none | Docker daemon passthrough | none |
| ecs | AWS | ECS tasks | forward |
| lambda | AWS | Lambda invocation | reverse |
| cloudrun | GCP | Cloud Run jobs | forward |
| gcf | GCP | Cloud Functions invocation | reverse |
| aca | Azure | Container Apps jobs | forward |
| azf | Azure | Azure Functions invocation | reverse |

### Simulators (3)

| Simulator | SDK | CLI | Terraform |
|---|---|---|---|
| AWS | 42 | 26 | 26 |
| GCP | 43 | 21 | 20 |
| Azure | 38 | 19 | 29 |

## Known Limitations

1. **FaaS transient failures** — ~1 per sequential E2E run on reverse agent backends
2. **Upstream act individual mode** — azf requires `--individual` flag
3. **Azure terraform tests** — Docker-only (Linux); macOS ignores `SSL_CERT_FILE`
