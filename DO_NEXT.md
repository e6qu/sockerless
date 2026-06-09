# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `bundled-docs-bigtable-execution`.

This branch bundled three tasks:

- GCP Bigtable Terraform coverage was added to `simulators/gcp/terraform-tests/main.tf` with apply-test assertions for the provider resource IDs. The follow-up CI failure showed that the Google provider used the official Bigtable gRPC emulator path, so the simulator now served Bigtable Admin over gRPC and the Terraform harness exported `BIGTABLE_EMULATOR_HOST`.
- AWS Batch, CodeBuild, Glue, and Step Functions stopped returning synthetic terminal success. Batch ran real containers; CodeBuild ran buildspec shell commands; Glue ran Python shell scripts loaded from S3; Step Functions ran supported ASL states and aborted running Wait executions.
- Continuity docs, BUG accounting, the coverage matrix, and service surface tables were refreshed and cross-linked.

## Verification Done

- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run '^$' ./...` in `simulators/aws`
- Targeted AWS SDK tests for Batch, CodeBuild, Glue, and Step Functions passed with Docker access.
- Targeted AWS CLI tests for Batch, CodeBuild, Glue, and Step Functions passed with Docker/AWS CLI access.
- GCP Terraform Bigtable coverage passed through a focused provider apply/destroy regression, and the full GCP Terraform test module passed locally. The main apply stack remained a Linux CI path because the local macOS harness skips it.

## Before Opening The PR

1. Run the final guard scripts and targeted Terraform test again after the docs are complete.
2. Rebase on `origin/main`.
3. Push the branch and create one PR.
4. After pushing, sync local `main` with `origin/main`.

## Next Work After This PR

- Re-check `gh issue list --state open --limit 30`; #394 was the only open issue at the start of this branch and remained upstream-blocked.
- Continue simulator fidelity audits from real SDK/CLI/Terraform behavior, especially any row still marked `not applicable` or lacking one of the three public-client surfaces.
- Keep BUG-1075 open until authenticated live-cloud validation runs exist.
- Keep BUG-1104 open as the standing audit-cadence tracker.

## Session Checklist

1. `git fetch origin main`
2. `git checkout main`
3. `git pull origin main`
4. `gh issue list --state open --limit 30`
5. Check [BUGS.md](BUGS.md) counters and open rows.
6. Create a fresh branch from current `main`.
7. File a BUG before fixing any newly discovered defect.
8. Run affected SDK, CLI, Terraform, and guard tests before pushing.

## Rules That Mattered Most

- No synthetic success paths. Returning a successful cloud state required real work or a real cloud-shaped state transition.
- No fallback behavior. Missing executable inputs produced explicit API errors or failed runs.
- Simulator docs were treated as source of truth: when a test or implementation changed, [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and the matching file under [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES) changed too.
