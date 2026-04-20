# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs/CLIs/Terraform.

86 phases, 757 tasks, 707 bugs tracked (all fixed). See [STATUS.md](STATUS.md), [BUGS.md](BUGS.md), [specs/](specs/).

## Phase 86 — Simulator parity + Lambda agent-as-handler

Closes the Phase 86 plan: every cloud-API slice sockerless depends on is now a first-class cloud-slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or an explicit exemption). Lambda's agent-as-handler pattern for `docker exec` / `attach` is fully wired up: bootstrap loop + overlay image build + reverse-agent WebSocket server.

Branch: `phase86-complete-runner-support` → PR #112 (merged 2026-04-20 as commit `7f054e0`).

Phase C — live-AWS session 2 — is in progress on `post-phase86-continuation` off `origin/main`. Session 1's two blocker bugs (BUG-692 docker-run hang, BUG-P86-A2 raw ECS image ref) are fixed; session 2 reruns the full runbook (0-infra-up → 6-teardown) + the e2e runner matrix. Plan at `~/.claude/plans/purring-sprouting-dusk.md`.

### Simulator parity (AWS + GCP + Azure)

- **A.5** — Pre-commit testing contract enforced in `.pre-commit-config.yaml` + `AGENTS.md`: every `r.Register("X", ...)` addition needs a matching SDK + CLI + terraform-tests entry, or an explicit opt-out in `simulators/<cloud>/tests-exempt.txt`.
- **BUG-696** — AWS ECR pull-through cache slice (`CreatePullThroughCacheRule` / `DescribePullThroughCacheRules` / `DeletePullThroughCacheRule` + URI rewriting).
- **BUG-697** — `Store.Images` persistence across backend restart (six cloud backends; default path `~/.sockerless/state/images.json`).
- **BUG-700** — Cloud-side network-create failures surface as response `Warning` on ECS, Cloud Run, and ACA (was silently dropping DNS + security-group errors).
- **BUG-701** — Cross-task DNS via real Docker networks: AWS Cloud Map namespaces, GCP Cloud DNS private zones, Azure ACA environments. Shared-library helpers `EnsureDockerNetwork` / `ConnectContainerToNetwork` in each cloud's `shared/container.go`.
- **BUG-702** — Azure Private DNS Zones backend SDK wire (`armprivatedns`). In-memory `serviceRegistry` removed entirely.
- **BUG-703** — Azure NSG backend SDK wire (`armnetwork/v7`) + simulator `securityRules` sub-resource CRUD consistent with the NSG's `Properties.SecurityRules` array.
- **BUG-704** — GCP Cloud Build slice with real `docker build` execution, LRO polling, streaming logs.
- **BUG-705** — AWS Lambda Runtime API slice: per-invocation HTTP sidecar on `127.0.0.1:<port>` handling `/next`, `/response`, `/error`, `/runtime/init/error`. Container-to-host via `host.docker.internal`.
- **BUG-706** — Azure ACR Cache Rules slice (cacheRule CRUD + pull-through URI rewriting via `ResolveAzureImageURIWithCache`).
- **BUG-707** — GCP Cloud Build Secret Manager integration (availableSecrets → runtime env var resolution via the new `simulators/gcp/secretmanager.go` slice).
- **GCP Cloud Run v1 services** — Knative-style CRUD for parity completeness.

See `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md` for the complete slice matrix. Zero ✖ rows on the runner path.

### Phase D — Lambda agent-as-handler

- **D.1** `agent/cmd/sockerless-lambda-bootstrap/main.go` — real Runtime-API polling loop. Parses `Lambda-Runtime-*` headers, spawns user entrypoint + CMD with invocation payload on stdin, posts `/response` (or `/error` envelope). Reverse-agent WebSocket dialed once at init with 20s heartbeat.
- **D.2** `backends/lambda/image_inject.go` — `BuildAndPushOverlayImage` renders the overlay Dockerfile, stages agent + bootstrap binaries, runs `docker build` + `docker push` to the destination ECR URI. `ContainerCreate` calls it when `CallbackURL` is set.
- **D.3** `backends/lambda/reverse_agent_server.go` — WebSocket upgrade at `/v1/lambda/reverse?session_id=...` mounted on the BaseServer mux. `reverseAgentRegistry` handles register/resolve/drop with reconnect-same-session-id resume semantics.
- **D.4** `lambdaExecDriver` + `lambdaStreamDriver` route `docker exec` / `docker attach` through the reverse-agent session. Real end-to-end test at `backends/lambda/agent_e2e_integration_test.go` (gated on `SOCKERLESS_INTEGRATION=1`): builds the real bootstrap + bakes into a test image; runs real docker + AWS simulator + Lambda backend; `docker run` → Lambda invoke → sim spawns handler → bootstrap dials back via `host.docker.internal` → `docker exec` resolves via `lambdaExecDriver` → bootstrap spawns subprocess → stdout returns. Passes in ~1.5s. Post-stop path verified too.

### CI codification

- `scripts/phase86/0-infra-up.sh` through `6-teardown.sh` — idempotent shell scripts for each runbook.
- `.github/workflows/phase86-aws-live.yml` — `workflow_dispatch`-only with sensitive tokens as inputs. Teardown runs under `if: always()` so a failed earlier job still releases scratch AWS resources.

### Live-AWS (Phase E)

Awaiting AWS credentials. The `runner-capability-matrix.md` live columns stay pending-live until the workflow is dispatched successfully.
