# Sockerless — Current Status

**Phase 82 complete. 725 tasks done across 80 phases. 527 bugs fixed (41 sprints). 0 open bugs.**

## Test Results

| Category | Count |
|---|---|
| Core unit tests | 302 PASS (`cd backends/core && go test -race -v ./...`) |
| Frontend tests | 7 PASS (TLS + mux) |
| UI tests (Vitest) | 92 PASS (13 packages) |
| Admin tests | 88 PASS |
| Admin Playwright E2E | 17 PASS |
| SPAHandler tests | 5 PASS |
| bleephub | 304 unit + 9 integration + 1 gh CLI |
| gitlabhub | 136 unit + 17 integration |
| Sandbox | 46 PASS |
| Shared ProcessRunner | 15 PASS |

### E2E & Integration

| Suite | Count | Command |
|---|---|---|
| Sim-backend integration | 75 PASS | `make sim-test-all` |
| GitHub E2E | 217 PASS (31 × 7 backends) | `make e2e-github-{backend}` |
| GitLab E2E | 154 PASS (22 × 7 backends) | `make e2e-gitlab-{backend}` |
| Upstream gitlab-ci-local | 252 PASS (36 × 7 backends) | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS | `make tf-int-test-all` |
| Cloud SDK tests | AWS 42, GCP 43, Azure 38 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 26, GCP 21, Azure 19 | `make docker-test` per cloud |
| Lint (19 modules) | 0 issues | `make lint` |

## Architecture

### Backends (8)

| Backend | Cloud | Execution | Agent |
|---|---|---|---|
| memory | none | WASM sandbox (in-process) | none |
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

1. **GitLab + memory = synthetic** — gitlab-runner helper binaries can't run in WASM
2. **WASM sandbox scope** — No bash/node/python/git; busybox + 21 Go builtins + POSIX shell only
3. **FaaS transient failures** — ~1 per sequential E2E run on reverse agent backends
4. **Upstream act individual mode** — memory and azf require `--individual` flag
5. **Azure terraform tests** — Docker-only (Linux); macOS ignores `SSL_CERT_FILE`
