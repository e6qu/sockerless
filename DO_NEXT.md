# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/faithful-registry-roundtrip-1785` (PR pending) — **BUG-1785 azure half: faithful build→push→pull for ACR Tasks.** The sim's ACR Tasks now does a real `docker push` to the registry + `docker rmi`; the ACA App run does a real `docker pull` — registry and compute agnostic (only `/v2/`). Backend honors `SOCKERLESS_AZURE_ACR_ENDPOINT`; harness publishes the sim `/v2/` at `127.0.0.1:5000` + a podman-machine insecure drop-in. Validated by ACA harness TEST 12 + the ACR Tasks SDK test.

### Remaining work

1. **BUG-1785 gcp half.** Carry the same faithful push/pull through the gcp Cloud Build slice (`simulators/gcp/cloudbuild.go`'s confirmed-local push → real `docker push` + `rmi`) AND the cloudrun/gcf overlay flows: add a GCP AR registry-endpoint override (parallel to `SOCKERLESS_AZURE_ACR_ENDPOINT`) and update the `cloudrun`/`cloudrun-functions` integration tests + the gcp `build_test.go` (which today rely on the local-daemon shortcut and would break on a real push to an unreachable AR). Larger + higher-risk than the azure half — do it carefully with a reachable registry stand-in.
2. **TEST 13 — ACA service container (BUG-1784).** The job container's `curl http://<service-alias>` exits 1: the sibling service App's alias doesn't resolve from inside the job App. Wire ACA cloud-DNS / per-job-network service discovery.
3. **TEST 14 — dispatcher-spawned runner.** `Connection refused (host.docker.internal:80)` — a published-port / external-URL wiring detail.
4. **Cloud Run + GCF topology cells** — same `cloudrun-bootstrap` overlay model.

### Reusable finding (registry round-trip)
A real `docker push`/`pull` to the sim registry needs the host engine to trust it. **Docker auto-trusts loopback registries; Podman does not** — so the harness publishes the sim `/v2/` at `127.0.0.1:5000` and the ACA Make target drops a scoped insecure entry on the podman machine. On Docker / Linux CI it's a no-op. The backend points the image ref at that reachable endpoint via `SOCKERLESS_AZURE_ACR_ENDPOINT` (a legit custom-cloud override), keeping the sim's registry and compute services agnostic.

### Reusable findings (this branch)
- ACA container-job exec needs the **App overlay** (`SOCKERLESS_ACA_USE_APP=1`) + an ACR-Tasks-built bootstrap image; the sim builds it on the host engine and runs it by local tag.
- The bootstrap/agent **must be statically linked** (`CGO_ENABLED=0`) to exec in musl/alpine/scratch overlays.
- Sim storage-over-HTTP needs a resolvable advertised endpoint: `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` pins `<account>.blob.localhost`, plus an `/etc/hosts` alias inside the harness container (`*.localhost` is not special-cased by the container resolver).

## Next (pod model + runner integration focus)

Grounded gap matrix: only **Lambda** is live-proven (BUG-1075); the GitHub container-job topology is **sim-proven for ECS only**; the ACA cell got past networking + lifecycle (BUG-1780) but container-job exec needs the bootstrap overlay; the FaaS backends can't yet assemble multi-container pods (BUG-1781).

- **Arc 2 — GitHub topology harness sweep (ACA → Cloud Run → GCF).** Land the harness plumbing (multi-backend image, `BLEEPHUB_BACKEND` parameterization — preserved in `/tmp/aca-harness-wip/`) and finish the ACA cell: container-job exec needs the reverse-agent bootstrap injected via the ACA App overlay (`SOCKERLESS_ACA_USE_APP=1` + an ACR-Tasks build). **Build this through faithful cloud APIs only** — the azure sim implements real ACR Tasks/Registry semantics and the host engine pulls the overlay as a real client would; no sockerless-aware sim hook. Then Cloud Run + GCF (same `cloudrun-bootstrap` overlay model). Turns "ECS sim-proven" into "all container backends sim-proven."
- **FaaS multi-container pod assembly (BUG-1781).** Assemble pod semantics from cloud primitives on lambda/gcf/azf (sidecars where offered, else a pod from multiple functions + cloud DNS + shared volume, agent proxying localhost to siblings) so `services:`/sidecar `container:` jobs run there; replaces the interim fail-fast rejections.
- **Arc 3 — GitLab docker-executor topology parity** — a sim-backed harness proving the full helper + build + service-container flow across backends.
- **Live pass (BUG-1075)** — once the above are sim-proven, the live run against real ECS/Lambda/etc. (user-gated spend).

Other standing candidate: issue #363 (versioned releases + GHCR). Re-check `gh issue list --repo e6qu/sockerless` before consumer-issue work; only #394 (azuread, upstream-blocked) is open.

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
