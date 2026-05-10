# Do Next

Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Branch

`state-save-post-pr139` — PR #140 (Phase 81 + Phase 82 + state save post-#139) awaiting CI / review / merge. Once merged, start a new branch for Phase 83.

## Resume here — Phase 83 (sim UI parity)

**State of the sim UIs today** (from `ui/packages/simulator-{aws,gcp,azure}/`):

- Shell parity is already there — `SimulatorApp` (`@sockerless/ui-core/components`) wraps `ErrorBoundary` + `ToastProvider` + `BrowserRouter` + `AppShell` (which itself includes `ThemeToggle` in the nav). So shell, error boundary, toast plumbing, dark mode all exist.
- Page parity is the gap — the sim pages are 20–30 LOC each (`<h2>` + `<DataTable>`), no `PageHeading` / kicker style, no error UX, no toast wiring on (currently nonexistent) mutations. Compared to the admin pages they look like sketches.

**Phase 83 deliverables, ordered.**

1. **Sim page polish.** Sweep every sim page (`simulator-aws`: ECSTasks, Lambda, ECR, S3, LogGroups, Overview; `simulator-gcp`: ArtifactRegistry, CloudFunctions, CloudRunJobs, GCSBuckets, Logging, Overview; `simulator-azure`: ACRRegistries, AzureFunctions, ContainerApps, Monitor, StorageAccounts, Overview). Replace bare `<h2>` with `PageHeading` (kicker like `aws · simulator · ecs`, italic `<>Tasks</>` title, meta showing row count). Add `ErrorPanel` for `isError` paths (currently silently render empty). Match the admin design language: card-style sections, font-display titles, font-mono uppercase tracking-[0.18em] kickers.
2. **Lift reusable bits to `@sockerless/ui-core`.** The four 25-LOC sim pages all do the same thing: `useQuery` + `DataTable` + nothing. Extract a `<ResourceListPage>` shared component that takes `{ kicker, title, columns, queryKey, queryFn, refetchInterval, emptyMessage }` and emits the heading + spinner + error + table. Each sim page collapses to a config call. Net code drop > add even after the polish.
3. **Reuse Phase 81 surface for sims.** SSE log tail and API console at `/ui/topology/:p/:i/logs` + `/ui/topology/:p/console` already accept any topology instance regardless of kind, so sims get them free — but only when launched via admin orchestration. Sims launched by `make stack-X-Y` register in `sockerless.yaml`, so this is the working path; document it.
4. **Retire legacy pages once topology covers them.**
   - `/ui/resources` (legacy registry-backed) — superseded by `/ui/topology/resources`. Delete the route + page once the new one is verified in operator workflow.
   - `/ui/projects/:name/logs` (legacy combined-component logs) — superseded by `/ui/topology/:p/console` (combined timeline). Delete after the same verification.
   - `cmd/sockerless-admin/api_components.go` and the legacy registry path can stay; it's still useful for components added via CLI flag (`--backend name=addr`) that bypass `sockerless.yaml`. The UI nav just stops linking to the legacy pages.
5. **Document.** `ui/README.md` gets a "Sim UIs" section pointing operators at the per-sim cloud-API browser pages; admin orchestration docs note that sim instance logs / console are reachable from `/ui/topology/...`.

**Out of scope for Phase 83.** Don't add Containers / Resources / Metrics pages to sims — those are *backend* concepts (Docker container lifecycle, sockerless-tracked cloud resources, backend metrics). Sims model the cloud APIs (ECS tasks, Lambda functions, S3 buckets) directly; those domain pages already exist and just need polish.

**Quick start when picking up.**

```bash
git checkout main && git pull
git checkout -b phase-83-sim-ui-parity
cd ui/packages/simulator-aws/src/pages
# inspect ECSTasksPage.tsx alongside admin's TopologyResourcesPage.tsx
# the latter is the design target, the former is the starting point
```

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). For Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's stdout behaviour.
- **No fallbacks.** Unknown config values fail-loud. No silent defaults.
- **CI green per commit.** Each commit must be independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` (no skip).
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.

## Roadmap

Phases 79.2 → 80 → 81 → 82 ✓ all in #140. Next: 83 → 84 → 85 → 86 → 87. After 87: 91–94 (real per-cloud volume provisioning) + the live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live). See [PLAN.md](PLAN.md) for sub-steps. Will likely split into multiple PRs once natural seams appear (e.g. Phase 87 — observability — is independent and can land standalone).
