# Sockerless — Current Status

**Phase 68 (Multi-Tenant Backend Pools) in progress. 595+ tasks done across 71 phases.**

## Test Results (Latest)

### Summary

| Category | Count |
|---|---|
| **Core unit tests** | 255 PASS (`cd backends/core && go test -race -v ./...`) — includes 3 OTel + 14 network driver/IPAM tests |
| **Frontend tests** | 4 PASS (TLS) + 3 PASS (mux) |
| **bleephub** | 298 unit + 9 integration + 1 gh CLI (35 assertions) — includes 5 OTel tests |
| **gitlabhub** | 129 unit + 17 integration |
| **Sandbox** | 46 PASS |
| **Shared ProcessRunner** | 15 PASS (5 × 3 clouds) |

### E2E & Integration

| Suite | Count | Command |
|---|---|---|
| Sim-backend integration | 129 PASS | `make sim-test-all` |
| GitHub E2E | 31 workflows x 7 backends = 217 PASS | `make e2e-github-{backend}` |
| GitLab E2E | 22 pipelines x 7 backends = 154 PASS | `make e2e-gitlab-{backend}` |
| Upstream gitlab-ci-local | 36 tests x 7 backends = 252 PASS | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS (ECS 21, Lambda 5, CR 13, GCF 7, ACA 18, AZF 11) | `make tf-int-test-all` |
| Cloud SDK tests | AWS 35, GCP 36, Azure 31 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 24, GCP 19, Azure 17 | `make docker-test` per cloud |
| Lint (15 modules) | 0 issues | `make lint` |

### Cloud SDK Test Breakdown (Phase 70-71 additions)

| Cloud | Before P70 | After P70 | After P71 | P71 new tests |
|---|---|---|---|---|
| AWS ECS | 4 | 8 | 8 | — |
| AWS Lambda | 13 | 13 | 27 | `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsToCloudWatch` |
| GCP Cloud Run | 8 | 11 | 11 | — |
| GCP Cloud Functions | 12 | 12 | 25 | `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsRealOutput` |
| Azure ACA | 7 | 10 | 10 | — |
| Azure Functions | 9 | 9 | 21 | `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsRealOutput`, `DefaultHostNameReachability` |

### Cloud CLI Test Breakdown (Phase 71 additions)

| Cloud | Before P71 | After P71 | P71 new tests |
|---|---|---|---|
| AWS | 21 | 24 | `TestECS_CLI_RunTaskAndCheckLogs`, `TestECS_CLI_RunTaskNonZeroExit`, `TestLambda_CLI_InvokeAndCheckLogs` |
| GCP | 15 | 19 | `TestCloudRun_CLI_RunJobAndCheckLogs`, `TestCloudRun_CLI_RunJobFailure`, `TestFunctions_CLI_InvokeAndCheckLogs`, `TestFunctions_CLI_Delete` |
| Azure | 14 | 17 | `TestContainerApps_CLI_StartAndCheckLogs`, `TestContainerApps_CLI_StartFailure`, `TestFunctionApp_CLI_InvokeAndCheckLogs` |

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
| AWS | 35 | 24 | 26 |
| GCP | 36 | 19 | 20 |
| Azure | 31 | 17 | 29 |

## Known Limitations

1. **GitLab + memory = synthetic** — gitlab-runner helper binaries can't run in WASM
2. **WASM sandbox scope** — No bash/node/python/git; busybox + 21 Go builtins + POSIX shell only
3. **FaaS transient failures** — ~1 per sequential E2E run on reverse agent backends
4. **Upstream act individual mode** — memory and azf require `--individual` flag
5. **Azure terraform tests** — Docker-only (Linux); macOS ignores `SSL_CERT_FILE`
