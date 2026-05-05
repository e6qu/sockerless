# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v31 — cell 8 v8 in flight; cells 5+6 not started)

User goal (recorded today): **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 done; cells 5/6/8 outstanding.

### Cell 8 — currently in flight

Cell 8 v7 evidence (https://gitlab.com/e6qu/sockerless/-/jobs/14219764410):
- ✅ prepare_executor 52 s (was 90 s before parallelization)
- ✅ prepare_script 65 s
- ❌ get_sources: `Container not found` from docker exec lookup at 0 s

Diagnosis: pod was deployed as multi-container Cloud Run Service (BUG-953 fix) successfully — confirmed via `gcloud run services describe sockerless-svc-c1c74a8e1dfc`: it carries `sockerless_managed=true` label + `sockerless_pod_members` annotation listing both build + postgres container IDs. But `cloud_state.queryPodServiceContainers` doesn't surface it — possibly because the gRPC `ListServices` response abbreviates `Annotations`.

**v8 fix shipped (commit 16f3eb6, rev `00039-rbj`, sha256:c69b3711):** when a sockerless-managed Service has the right name shape (`sockerless-svc-*`) but empty annotations in the list response, do a `GetService` follow-up to fetch the full proto. Also adds INFO-level diagnostic logging on every pod-service match so the next iteration can confirm via Cloud Logging which path fired.

Cell 8 v8 trigger pipeline already in flight via the `gitlab-cell-8-test` branch.

### If cell 8 v8 is GREEN

1. Update STATUS.md / WHAT_WE_DID.md / BUGS.md to mark BUG-953 closed.
2. Move to cells 5+6.

### If cell 8 v8 still fails

Most likely failure modes:
- `queryPodServiceContainers` still doesn't match — check the new diagnostic INFO log; if absent, the iterator never hits sockerless-managed Services (could be paging issue).
- ContainerExec via Path B fails because the bootstrap was deployed without the right env (entrypoint/cmd not propagated). Check sockerless logs for `post exec envelope` errors.
- ContainerStart's resolveGCFFromCloud returns the wrong URL (e.g. main vs sidecar member resolution).

### Cells 5 + 6 — dispatcher refactor (not yet started)

The github-runner-dispatcher-gcp currently spawns one custom-image Cloud Run Job per workflow_job (sockerless baked into runner image — OLD architecture).

**Required refactor**: dispatcher creates pre-deployed Cloud Run Job per cell label with multi-container TaskTemplate:

1. **Container 1 (vanilla `actions/runner --ephemeral`):** GitHub registration via `RUNNER_REG_TOKEN` + `RUNNER_NAME` + `RUNNER_LABELS` env. Polls GitHub for one workflow, runs it, exits.
2. **Container 2 (sockerless-backend-cloudrun for cell 5 / sockerless-backend-gcf for cell 6):** standalone backend, listens on :3375 / :3376.

The runner has `DOCKER_HOST=tcp://localhost:3375|:3376`; the runner's docker calls (services: clauses, container steps) go through it.

**Implementation outline**:
- `github-runner-dispatcher-gcp/internal/spawner/spawner.go::Spawn` — extend `Request` with `SockerlessImage`, `BackendPort`. Replace single-container `containerCfg` with multi-container TaskTemplate.
- `github-runner-dispatcher-gcp/internal/config/config.go::Label` — add `sockerless_image` and `backend_port` toml fields.
- Operator config: per-label entries (`sockerless-cloudrun`, `sockerless-gcf`) point at the right backend image + port.
- Build + push a NEW dispatcher image; redeploy `github-runner-dispatcher-gcp` Cloud Run Service.
- Trigger cells 5+6 via `gh workflow run cell-5-cloudrun.yml` and `gh workflow run cell-6-gcf.yml`.

Cell 6 inherits cell 8's gcf stack — once cell 8 is GREEN, cell 6 should follow with this dispatcher refactor.

### Single-line summary

> Cell 7 GREEN, cell 8 v8 in flight (BUG-953 structural fix shipped — pod-mode now uses direct multi-container CR Service deploy instead of Cloud Functions wrapper). Next: verify cell 8 GREEN, then refactor github-runner-dispatcher for cells 5+6.
