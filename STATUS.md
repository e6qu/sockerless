# Sockerless — Status

**Where we are right now.** Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-121b-azure-sim-hardening` (off `origin/main` at 3e39e3a) |
| Last merged | PR #134 — Phase 127 storage driver expansion (2026-05-09) |
| Milestone | **8/8 runner-integration cells GREEN** (since 2026-05-07); **sim host model shipped, CI fully green on native arm64** (Phase 135, 2026-05-09). |
| Bugs | 984 filed · 984 fixed · **0 open** ([BUGS.md](BUGS.md)). |
| Sim parity | 77/77 ✓ across current backends (AWS 33, GCP 16, Azure 28) — [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) |
| Live infra | None up. All projects torn down end of 2026-05-07. |

## Cell scoreboard

| Cell | Stack | State | URL |
|------|-------|-------|-----|
| 1 | GH × ECS | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25075259911) |
| 2 | GH × Lambda | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25113565115) |
| 3 | GL × ECS | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177) |
| 4 | GL × Lambda | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943) |
| 5 | GH × Cloud Run | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792865) |
| 6 | GH × GCF | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792937) |
| 7 | GL × Cloud Run | GREEN v54 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14237010667) |
| 8 | GL × GCF | GREEN v28 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14234857458) |

Each green run: probe-capabilities → probe-localhost-peer (postgres sidecar `localhost:5432`) → clone-and-compile (`git clone` + `go build` of `simulators/testdata/eval-arithmetic`) → 5 arithmetic invocations.

## Up next on `phase-121b-azure-sim-hardening`

In flight: **Phase 121b — Azure simulator hardening** (mirror of Phase 121 GCP work). Two cloud-faithful upgrades shipped:

- **121b-A — Azure Files data plane on disk.** `simulators/azure/files.go`'s data-plane handler previously returned mock XML for every file/directory operation. New `handleAzureFilesPath` services the real Azure Files REST verbs (PUT directory, PUT file, PUT range, GET, HEAD, DELETE) and persists everything under `FileShareHostDir(account, share)` — the same on-disk root the ACA Jobs/Apps executor uses for `Volume{StorageType: AzureFile}`. End-to-end consistency between data-plane writers and workload mounts.
- **121b-B — HS256-signed Azure AD JWT.** `simulators/azure/auth.go` previously emitted `alg:none` tokens. New `mintAzureSimJWT` produces a real-shape Azure AD access token (HS256 + `kid` header + `tid`/`oid`/`sub`/`aud`/`iss`/`iat`/`exp`/`nbf`/`ver`/`appid` claims). JWKS publishes the `kid`. Mirrors GCP sim's HS256 approach.
- **121b-C — All 6 backends' integration test harness restructured.** No more `SOCKERLESS_INTEGRATION` env-var gate, no skip, no fallback, no `//go:build integration` build tag. Every backend's `TestMain` (ACA, AZF, ECS, Lambda, Cloud Run, Cloud Run Functions) now requires `SOCKERLESS_TEST_TARGET=sim|cloud` and `docker` + `go` on PATH; sim path builds + starts the per-cloud simulator on a free port and pre-creates the fixed sim fixtures; cloud path requires explicit `SOCKERLESS_ENDPOINT_URL` + per-backend ARM/IAM env vars. The `Test*` functions are target-agnostic — they hit the docker SDK regardless. Per-test `t.Skip(SOCKERLESS_INTEGRATION...)` and `skipIfNoIntegration(t)` helpers are deleted. Fixes the prior nil-`dockerClient` panic on local-dev `go test`.
- **121b-D — Azure terraform-test darwin fail-loud.** `simulators/azure/terraform-tests/apply_test.go` no longer skips on darwin — it `t.Fatal`s with a clear explanation (Go's cgo `crypto/x509.SystemCertPool()` reads from the macOS Security framework keychain and ignores `SSL_CERT_FILE`; run via Linux container or in CI).
- **121b-E — Makefile + CI updates.** `make/go-app.mk` + `make/go-lib.mk` ship `test-integration` (sets `SOCKERLESS_TEST_TARGET=sim`) + `test-integration-cloud` (sets `=cloud`). CI sets `SOCKERLESS_TEST_TARGET=sim` only — the legacy `SOCKERLESS_INTEGRATION` env var is removed entirely from the codebase.

- **121b-F — In-memory storage backing driver.** New `core.MemoryDriver` (cloud-agnostic; sibling to `EmptyDirDriver`). `BackingMemory = "memory"` constant + `MemorySpec{ SizeMB int }` payload + `SharedVolumeRef.MemorySizeMB` field. Registered at startup across all 6 backends — operators select via `SharedVolume.Backing="memory"`. Each backend's volume translator emits the cloud-native RAM-backed primitive (EmptyDir{Medium: MEMORY} on Cloud Run / GCF / ACA, ECS tmpfs, Lambda /tmp scratch). 5 unit tests.

Tests: 5 new sim unit tests (`files_test.go` × 3, `auth_test.go` × 2) + 5 core unit tests (`TestMemoryDriver_*`). All `simulators/azure` + `azure/sdk-tests` + `azure/cli-tests` + `azure/terraform-tests` green locally; every backend's integration test harness builds cleanly and fails loud on missing config.

Then Phase 78 per [PLAN.md § Roadmap (ordered)](PLAN.md#roadmap-ordered).

## Recently shipped (chronological)

| Date | PR | Headline |
|------|----|----|
| 2026-05-09 | #134 | Phase 127 Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral; per-cloud drivers + per-backend storageBackings registry wiring across all 6 cloud backends). |
| 2026-05-09 | #133 | Phase 126 Access driver (iam-role / id-token / mTLS / none-internal; per-backend adapters; every idtoken.NewClient callsite migrated through s.Access.AuthenticatedClient). |
| 2026-05-09 | #132 | Phase 125 DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none; per-backend adapters; SOCKERLESS_DNS_SEARCH_DOMAIN env wired through every ContainerCreate; cloudrun + GCF bootstraps write /etc/resolv.conf). |
| 2026-05-09 | #131 | Phase 124 network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only; per-backend adapters; all callsites migrated). |
| 2026-05-09 | #130 | Phase 128 runner job timeout (bootstrap timer + cloud-native cap; SOCKERLESS_JOB_TIMEOUT_SECONDS contract). |
| 2026-05-09 | #129 | Phase 135 sim host model + 3-tier coverage + native arm64 CI runners. 12 bugs closed (BUG-949/972/975-984). |
| 2026-05-08 | #128 | Makefile standardization + per-app leaf Makefiles + stack orchestration; 17 doc updates; BUG-973/974 sim test stability (`Eventually` polling). |
| 2026-05-08 | #127 | Phase 129 #4 orphan pod-Service GC (owner-link via `CLOUD_RUN_JOB`); sim parity prep (GCP `generateIdToken` + Compute Disks); Phases 130/131/132 (bleephub workflow runs / workflows / apps + oauth REST + UI dispatch + AppsPage + OAuthPage). |
| 2026-05-07 | #123 | Phase 123 storage-backing driver (`emptyDir` / `gcs-sync` / `gcs-fuse`); 4 GCP runner cells GREEN (5/6/7/8); milestone closure → 8/8 GREEN. |
| 2026-04-30 | #122 | Phase 110 — 4 AWS runner cells GREEN (1/2/3/4); 32 bug fixes (845–876). |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Open bugs (0)

All filed bugs closed. New CI / live-cloud failures land in [BUGS.md](BUGS.md) per the standing rule.
