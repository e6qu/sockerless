# Sockerless — Status

8/8 runner-integration cells GREEN (2026-05-07). Live infra torn down at end of that session. Currently on work branch `makefile-standardization` (PR #128): full Make target standardization across every leaf, README rewrite, 17 doc updates, sim test stability fixes (BUG-973/974). All 11 CI checks GREEN.

## Cell scoreboard

| Cell | Path | State | URL |
|------|------|-------|-----|
| 1 GH × ECS | sockerless-ecs | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25075259911) |
| 2 GH × Lambda | sockerless-lambda | GREEN | [run](https://github.com/e6qu/sockerless/actions/runs/25113565115) |
| 3 GL × ECS | sockerless-ecs | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177) |
| 4 GL × Lambda | sockerless-lambda | GREEN | [pipeline](https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943) |
| 5 GH × cloudrun | sockerless-cloudrun | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792865) |
| 6 GH × gcf | sockerless-gcf | GREEN v17 | [run](https://github.com/e6qu/sockerless/actions/runs/25506792937) |
| 7 GL × cloudrun | sockerless-cloudrun | GREEN v54 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14237010667) |
| 8 GL × gcf | sockerless-gcf | GREEN v28 | [job](https://gitlab.com/e6qu/sockerless/-/jobs/14234857458) |

Each green run: probe-capabilities/kernel/env/parameters → probe-localhost-peer (postgres sidecar on `localhost:5432`) → clone-and-compile (`git clone` + `go build` of `simulators/testdata/eval-arithmetic`) → 5 arithmetic invocations.

## On `makefile-standardization` (PR #128, all 11 checks GREEN)

1. **Phase 129 #4** ✅ — orphan `sockerless-svc-*` GC via `CLOUD_RUN_JOB` owner-link. Code shipped; live verification deferred.
2. **Sim parity prep** ✅ — GCP `iamcredentials.generateIdToken` (Phase 126) + Compute Disks CRUD (Phase 127). 8 new SIM_PARITY_MATRIX rows; 6 SDK tests PASS.
3. **Phase 130** ✅ — bleephub `actions/runs` + `actions/jobs` + `actions/runners` REST (10 routes). 14 new tests PASS.
4. **Phase 131** ✅ — bleephub `actions/workflows` REST (4 routes) + `WorkflowFile` entity with auto-discovery from git storage + auto-register on `/api/v3/bleephub/workflow` submit + UI Workflows/Runs tabs with dispatch dialog. 10 new Go tests + 4 new UI tests PASS.
5. **Phase 132** ✅ — bleephub apps + oauth completeness: `/api/v3/user/installations`, `/api/v3/user/installations/{id}/repositories`, `DELETE /api/v3/installation/token`, OAuth web flow. UI: AppsPage + OAuthPage. 14 new Go tests + 6 new UI tests PASS.
6. **Phase 133 — frontend design overhaul** ✅ — editorial-brutalist redesign (Fraunces serif + JetBrains Mono, sharp radii, per-app accent colors via Tailwind 4 `@theme`); 32 test assertions migrated to case-insensitive matchers across admin/bleephub/core (101/101 UI tests pass).
7. **Phase 134 — Makefile standardization** ✅ — `make/{help,colors,go-app,go-lib,ui-app,stack}.mk` shared includes; per-app Makefiles in every leaf; top-level path-based delegation (`make backends/ecs/build`); auto-generated help; stack orchestration (`make stack-aws-ecs`); 17 doc files updated; README rewrite around the new surface.
8. **BUG-973/974** ✅ — replaced fixed `time.Sleep(2s)` flakes in sim aws/azure SDK tests with `require.Eventually(30s, 250ms)` polling. Surfaced after the standardization unbroke the sim docker build that was masking these tests on main.

**Phase 130 milestone complete**: bleephub now offers the full GitHub API footprint the user named (workflows, apps, app installations, orgs already covered, OAuth web + device flows, runners). Live verification against `gh` CLI deferred to next live-cloud session.

## Resume

[DO_NEXT.md](DO_NEXT.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).
