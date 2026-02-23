# Sockerless — Current Status

**Phases 67+69 complete. 578 tasks done across 68 phases. Next: Phase 68.**

## Test Results (Latest)

### Summary

| Category | Count |
|---|---|
| **Core unit tests** | 255 PASS (`cd backends/core && go test -race -v ./...`) — includes 3 OTel + 14 network driver/IPAM tests |
| **Frontend tests** | 4 PASS (TLS) + 3 PASS (mux) |
| **bleephub** | 298 unit + 9 integration + 1 gh CLI (35 assertions) — includes 5 OTel tests |
| **gitlabhub** | 129 unit + 17 integration |
| **Sandbox** | 46 PASS |

### E2E & Integration

| Suite | Count | Command |
|---|---|---|
| Sim-backend integration | 129 PASS | `make sim-test-all` |
| GitHub E2E | 31 workflows x 7 backends = 217 PASS | `make e2e-github-{backend}` |
| GitLab E2E | 22 pipelines x 7 backends = 154 PASS | `make e2e-gitlab-{backend}` |
| Upstream gitlab-ci-local | 36 tests x 7 backends = 252 PASS | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS (ECS 21, Lambda 5, CR 13, GCF 7, ACA 18, AZF 11) | `make tf-int-test-all` |
| Cloud SDK tests | AWS 17, GCP 20, Azure 13 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 21, GCP 15, Azure 14 | `make docker-test` per cloud |
| Lint (15 modules) | 0 issues | `make lint` |

### Core Test Breakdown (255 PASS)

| Area | Tests | Key coverage |
|---|---|---|
| Docker API | 74 | Container CRUD, list/filter, logs, export, commit, update, prune, compose compat |
| Networking | 27 | Network driver (create/connect/disconnect/prune/list), IPAM (subnet/IP/release/MAC), DNS hosts, service discovery, event bus |
| Build & Images | 31 | Dockerfile parsing (FROM/COPY/ENV/CMD/SHELL/STOPSIGNAL/VOLUME/HEALTHCHECK), image prune |
| Health & Restart | 22 | Health check lifecycle, race-free transitions, restart delay/policy |
| Volumes & Mounts | 18 | Auto-create, lifecycle, mounts population, tmpfs |
| Pods & Registry | 46 | Pod registry, API, deferred start, crash recovery, resource registry, tags |
| Infrastructure | 24 | Metrics, management API, context loader, auth, docker config, error responses |
| Concurrency | 8 | PruneIf, concurrent state operations (with `-race`) |
| Compose lifecycle | 13 | Full up/down cycle, volume persistence, name reuse, network cleanup |

### Upstream Act (Informational)

| Backend | PASS | FAIL | Mode |
|---|---|---|---|
| memory | 91 | 24 | individual |
| ecs | 56 | 31 | monolithic |
| lambda | 57 | 30 | monolithic |
| cloudrun | 54 | 33 | monolithic |
| gcf | 58 | 29 | monolithic |
| aca | 58 | 29 | monolithic |
| azf | 69 | 16 | individual |

Remaining failures: Node.js 16 required (16), networking/services (4), Docker build with RUN (2), shell edge cases (2).

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
| AWS | 17 | 21 | 26 |
| GCP | 20 | 15 | 20 |
| Azure | 13 | 14 | 29 |

## Known Limitations

1. **GitLab + memory = synthetic** — gitlab-runner helper binaries can't run in WASM
2. **WASM sandbox scope** — No bash/node/python/git; busybox + 21 Go builtins + POSIX shell only
3. **FaaS transient failures** — ~1 per sequential E2E run on reverse agent backends
4. **Upstream act individual mode** — memory and azf require `--individual` flag
5. **Azure terraform tests** — Docker-only (Linux); macOS ignores `SSL_CERT_FILE`
