# Sockerless — Status

**Date: 2026-05-03 v16**. PR #123 (`phase-118-faas-pods`) — BUG-937 (3-stage AR-auth chain) shipped end-of-session. Cell 7 v33 (commit `a42ab93`) progressed from a 6-second 401-error to an 11-minute live execution that pulled all images, deployed BUILD container via Cloud Run Service, ran per-stage helper containers — confirming Phase 122g + the AR-auth fix work end-to-end. New blocker is **BUG-925** (postgres-on-Cloud-Run-Service): the `gitlab-runner-helper health-check` container failed with `No HOST or PORT found` because (a) Cloud Run only exposes HTTPS:443, never TCP:5432, and (b) gitlab-runner's `WAIT_FOR_SERVICE_TCP_*` env injection assumes the Docker --link / network-alias pattern. Cell 7 v33 cancelled at 11min. Cells 5/6/8 untouched this session.

## BUG-937 fix (3 commits on `phase-118-faas-pods`)

| Commit | What |
|---|---|
| `7410f11` | cloudrun + gcf `ImagePull` discard caller auth when ref is rewritten to AR proxy (basic-auth scoped to `registry.gitlab.com`/Docker Hub fails against AR) |
| `bb3412e` | (1) `core/registry.go::getRegistryToken` adds GCP/AR fast-path — strip `Bearer ` prefix and skip the Docker www-auth dance (AR accepts the OAuth2 access token directly); (2) cloudrun + gcf `ImagePull` aliases the freshly-pulled image under the original ref via `core.StoreImageWithAliases` so subsequent `/images/{ref}/json` resolves |
| `aa03bae` | BUGS.md log entry |

Live evidence (cell 7 v33 trace at https://gitlab.com/e6qu/sockerless/-/jobs/14191491097):
- `Pulling docker image registry.gitlab.com/.../gitlab-runner-helper:x86_64-v17.5.0` → `Using docker image sha256:180e3252... with digest us-central1-docker.pkg.dev/.../gitlab-registry/...` ✓
- `Pulling docker image postgres:16-alpine` → 5117ms ✓
- `Pulling docker image golang:1.22-alpine` → 2523ms ✓
- Cloud Build: 2× overlays built (cloudrun-d1bc978db... 78s, cloudrun-e543853a5e... 20s cached) ✓
- 4 per-stage Cloud Run Services deployed via overlay+bootstrap+Path-B HTTP envelope path ✓
- Postgres bootstrap log inside container: `database system is ready to accept connections` ✓

## What blocks cells 5-8 GREEN now

**BUG-925** is the remaining architectural wall. Three options under consideration:

1. **Cloud SQL** (managed Postgres) — drop the postgres service container; cell .gitlab-ci.yml connects to a Cloud SQL instance via UDS or private IP. Real-fix, but not docker-API-shaped (operator-managed external dep).
2. **Cloud Run multi-container sidecar** — postgres + bootstrap in same Cloud Run Service revision. Bootstrap proxies HTTP→TCP. Possible but Cloud Run sidecars are Preview and don't expose multiple ports externally.
3. **Trim postgres from cell-7/8 .gitlab-ci.yml** — focus the cell on what cloudrun integration actually validates (compile + use eval-arithmetic + probe environment); document postgres-as-side-car as out-of-scope. Pure runner+compile path.

User authorisation needed before picking. (3) is the fastest path to GREEN cells; (1) is the most complete fix. See `BUGS.md` BUG-925 for state.

## Original session notes preserved below


## Phase 122g + 122h shipped

Cell 7 progression captured in backend logs:
- v13 (BUG-928 PRIVATE_RANGES_ONLY) — bootstrap reached PORT 8080
- v14-v16 (BUG-929/931 series) — POST 200 + Service URL populated
- v20 (BUG-932 Cloud NAT + ALL_TRAFFIC) — `sockerless-cloudrun-bootstrap: subprocess argv=[/usr/bin/dumb-init /entrypoint gitlab-runner-helper cache-init /gitlab-runner-cache-init] exit=0` confirmed real cmd execution
- v21-v25 (BUG-934/935/936 wait+inspect fast-paths) — runner progressed to BUILD container (golang:1.22-alpine), POSTGRES service container UP listening on :5432, trace bytesize=28550
- v27 (Phase 122h stdinPipe + attach hijack) — code shipped (commit `9f9f872`); rev `00040-qj6` health probe failed; rolled back to `00038-f42`

## Source-code verified architectural findings (2026-05-03)

* gitlab-runner v17.5 `executors/docker/internal/exec/exec.go::defaultDocker.Exec`: HIJACKED `ContainerAttach(Stream+Stdin+Stdout+Stderr)` + `ContainerStart` + raw stdin-pipe per stage. NOT `/exec/...` API. Container reused via `StopKillWait`.
* github-runner v2.334.0 `src/Runner.Worker/Container/DockerCommandManager.cs`: `DockerExec` per step. Long-lived container `tail -f /dev/null`. Path B HTTP envelope maps directly.

ECS + Lambda already had `stdin_pipe.go` (~80 lines) for the gitlab-runner pattern — that's why cells 3+4 are GREEN. cloudrun + gcf were missing this. cloudrun's `AttachViaCloudLogs` is read-only — silently dropped script bytes.

## Live infra state

- `gitlab-runner-cloudrun` rolled back to rev `00038-f42` (pre-122h). Phase 122h rev `00040-qj6` health probe failed; needs debug.
- Cloud NAT `sockerless-nat` + Cloud Router `sockerless-router` provisioned (BUG-932).
- All long-lived Cloud Run Services have `ingress=internal`.
- CI lint `scripts/check-no-public-cloud-services.sh` enforces no `allUsers` invoker bindings.
- 13 new BUGs closed (922-936), 2 deferred (923 gcf timeout, 925 postgres DNS).

## Original baseline below — preserved for context.

## Cells 1-4 (AWS) — GREEN (Phase 110, closed 2026-04-30)
| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

## Cells 5-8 (GCP) — Phase 122g IN FLIGHT (overlay + Path B exec shipped)

**Phase 122g code-complete + deployed live (2026-05-03 v13)** — 7 commits:
1. `step 1`: lift `OverlayImageSpec` + renderer + tarball logic to `backends/gcp-common/image_inject.go`. gcf imports + sets prefix=`gcf-`; cloudrun uses same renderer with prefix=`cloudrun-`.
2. `step 2`: new `agent/cmd/sockerless-cloudrun-bootstrap` HTTP server (mirror of gcf bootstrap structure). Recognises envelope shape OR env-baked CMD. 7 unit tests pass. Runner image build pipeline (4 Dockerfiles + Makefiles) updated to bake the binary.
3. `step 2.5`: shared `ExecEnvelopeRequest/Response` + `PostExecEnvelope` helper in `gcp-common`. 5 unit tests pass.
4. `step 3a`: cloudrun ContainerCreate overlay-injects bootstrap when `SOCKERLESS_CLOUDRUN_BOOTSTRAP` env points at the binary. config.Image becomes the AR overlay URI; user.Entrypoint+user.Cmd ride as `SOCKERLESS_USER_*` env vars.
5. `step 3b/4`: cloudrun ExecStart Path B HTTP POST via `idtoken.NewClient` to Service URL. Reverse-agent WS reserved for interactive (TTY+stdin).
6. `step 5`: ContainerStart routes overlay-imaged containers to Cloud Run Service path (vs Job). New `useServicePath` helper triggers when image URI is in `sockerless-overlay/` AR repo.
7. `step 6`: gcf bootstrap extended with envelope shape; gcf ExecStart wired to Path B HTTP POST to Function URL.

**Live infra updates (2026-05-03 v13)**:
- AR images pushed: `runner:cloudrun-amd64` (rev `df504cdc…`), `gitlab-runner:cloudrun-amd64` (rev `ad45341c…`), `runner:gcf-amd64` (rev `18601a8e…`), `gitlab-runner:gcf-amd64` (rev `8431c6e7…`).
- Service redeployed: `gitlab-runner-cloudrun` rev `00023-dgh` + `gitlab-runner-gcf` rev `00020-5g8`.

**BUG-928 (Phase 122g new, fix shipped)**: cell 7 first 122g run progressed past auto-remove + start-cycle issues — Cloud Run Service WAS deployed for the gitlab-runner-helper permission container — but Cloud Run rejected with `terminated: Application failed to start; STARTUP TCP probe failed for port 8080`. Root cause: `VpcAccess_ALL_TRAFFIC` routes ALL container egress through the VPC connector subnet (10.8.0.0/28), which has no Cloud NAT — GCSFuse can't reach `storage.googleapis.com`, volume mount stalls, bootstrap never gets to bind PORT. Fix: switch to `VpcAccess_PRIVATE_RANGES_ONLY` (only RFC1918 traffic via connector; public Google APIs via platform egress). Shipped in `backends/cloudrun/{servicespec,jobspec}.go`.

**Cell 7 evidence chain**:
- v11 (pre-Phase-122g, post-BUG-922): pipeline 2496190828 SUCCESS but FAKE — zero workload markers in Cloud Logging.
- v12 (Phase 122g step 5 deployed): pipeline 2496241337 FAILED — Cloud Run Service deployed, but startup probe failed on port 8080 due to BUG-928 GCSFuse timeout.
- v13 (BUG-928 PRIVATE_RANGES_ONLY shipped): in flight at https://gitlab.com/e6qu/sockerless/-/pipelines/{TBD-after-monitor-completes}.

**Open architectural bugs**: BUG-923 (gcf CreateFunction.Wait — addressed by overlay pool reuse, awaiting verification), BUG-925 (postgres Service deploy + DNS — partly addressed by 122g, will revisit if cell 7 surfaces new evidence).

## Architectural state (specs/CLOUD_RESOURCE_MAPPING.md, 1219 lines)
Authoritative reference. Today's session updated:
- Lesson 6 REVISED for BUG-927 (overlay IS needed on cloudrun for stock images)
- Lesson 8 ADDED — Lambda's `execStartViaInvoke` Path B as the gcf+cloudrun adaptation pattern
- Synthesis section rewritten for Phase 122g scope

## Branch state
- `main` synced with `origin/main` at PR #121 merge.
- `phase-118-faas-pods` (PR #123) — 18+ commits this session; all CI green; ready for merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells GREEN; do NOT merge.
- `gitlab-cell-7-test`, `gitlab-cell-8-test` on `origin-gitlab` — pipelines for cells 7+8.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)
- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp`
- gitlab-runner-cloudrun (rev `00021-rzl` post BUG-922 fix), gitlab-runner-gcf
- VPC `sockerless-vpc` + subnet `sockerless-connector-subnet` (10.8.0.0/28) + connector `sockerless-connector`
- AR repos: `sockerless-live`, `docker-hub` (proxy), `gitlab-registry` (proxy), `sockerless-overlay/gcf`
- Secret Manager: `github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-build`, `sockerless-live-46x3zg4imo-runner-workspace`

See [PLAN.md](PLAN.md) for roadmap, [BUGS.md](BUGS.md) for bug log, [WHAT_WE_DID.md](WHAT_WE_DID.md) for narrative, [DO_NEXT.md](DO_NEXT.md) for resume runbook.
