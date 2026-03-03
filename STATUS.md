# Sockerless — Current Status

**Phase 82 (Admin Projects) complete. 725 tasks done across 80 phases. Bug Sprint 6 (BUG-043→046) fixed.**

## Test Results (Latest)

### Summary

| Category | Count |
|---|---|
| **Core unit tests** | 257 PASS (`cd backends/core && go test -race -v ./...`) — includes 3 OTel + 14 network driver/IPAM + 2 ProviderInfo tests |
| **Frontend tests** | 4 PASS (TLS) + 3 PASS (mux) |
| **UI tests (Vitest)** | 92 PASS (6 API client + 3 hooks + 5 DataTable + 2 BackendApp + 2 BackendInfoCard + 2 SimulatorApp + 7 LogViewer + 4 Admin DashboardPage + 4 Admin ProcessesPage + 3 Admin ProcessDetailPage + 4 Admin CleanupPage + 6 Admin ProjectsPage + 4 Admin ProjectCreatePage + 5 Admin ProjectDetailPage + 3 Admin ProjectLogsPage + 16 bleephub pages + 16 gitlabhub pages) |
| **Admin tests** | 88 PASS (83 existing + 2 RingBuffer Reset + 1 bootstrap error body + 1 RingBuffer Lines negative + 1 Stop/Start race) |
| **Admin Playwright E2E** | 17 PASS (dashboard, components, detail, containers, resources, metrics, contexts, navigation) |
| **SPAHandler tests** | 5 PASS (Go, in core) |
| **bleephub** | 304 unit + 9 integration + 1 gh CLI (35 assertions) — includes 5 OTel + 6 mgmt endpoint tests |
| **gitlabhub** | 136 unit + 17 integration — includes 7 mgmt endpoint tests |
| **Sandbox** | 46 PASS |
| **Shared ProcessRunner** | 15 PASS (5 × 3 clouds) |

### E2E & Integration

| Suite | Count | Command |
|---|---|---|
| Sim-backend integration | 75 PASS | `make sim-test-all` (6 backends × ~12 tests each) |
| GitHub E2E | 31 workflows x 7 backends = 217 PASS | `make e2e-github-{backend}` |
| GitLab E2E | 22 pipelines x 7 backends = 154 PASS | `make e2e-gitlab-{backend}` |
| Upstream gitlab-ci-local | 36 tests x 7 backends = 252 PASS | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS (ECS 21, Lambda 5, CR 13, GCF 7, ACA 18, AZF 11) | `make tf-int-test-all` |
| Cloud SDK tests | AWS 42, GCP 43, Azure 38 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 26, GCP 21, Azure 19 | `make docker-test` per cloud |
| Lint (19 modules) | 0 issues | `make lint` |

### Core Test Breakdown (255 PASS)

| Area | Tests | Key coverage |
|---|---|---|
| Docker API | 74 | Container CRUD, list/filter, logs, export, commit, update, prune, compose compat |
| Networking | 27 | Network driver, IPAM, DNS hosts, service discovery, event bus |
| Build & Images | 31 | Dockerfile parsing, image prune |
| Health & Restart | 22 | Health check lifecycle, race-free transitions, restart delay/policy |
| Volumes & Mounts | 18 | Auto-create, lifecycle, mounts population, tmpfs |
| Pods & Registry | 46 | Pod registry, API, deferred start, crash recovery, resource registry, tags |
| Infrastructure | 24 | Metrics, management API, context loader, auth, docker config, error responses |
| Concurrency | 8 | PruneIf, concurrent state operations (with `-race`) |
| Compose lifecycle | 13 | Full up/down cycle, volume persistence, name reuse, network cleanup |

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
