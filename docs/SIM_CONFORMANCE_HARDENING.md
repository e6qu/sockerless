# Simulator conformance + hardening — continuity doc

**Branch:** `feat/sim-conformance-hardening` (one PR, multiple staged commits; CI run per stage).
**Goal:** deep behavioural conformance of the AWS/GCP/Azure simulators against the *real* clients for the implemented slices, plus type + UI hardening. Find bugs → fix → regression-test → document.

This is the **resume artifact** for multi-session work. At the start of every session: read this doc + `BUGS.md` + the last 2 commits, then continue from the current stage's "Next" line.

## Conformance methodology (per surface)

Op-presence is already covered (95 surface tables in `specs/SIM_SURFACE_TABLES/`, SDK+CLI+Terraform per the coverage matrix, CI-enforced). This effort goes **deeper** — the recurring bug class (dropped fields, wrong list envelopes, missing top-level state, non-idempotent reads):

1. **Round-trip drift** — set every writable field via the real SDK/CLI, read it back, assert identical (catches dropped/renamed/defaulted fields).
2. **Idempotency** — `terraform plan -detailed-exitcode` after apply must be 0 (no perpetual diff).
3. **Error fidelity** — NotFound / Conflict / Validation paths return the real wire error code + exception shape the SDK classifies on.
4. **List/pagination fidelity** — envelope keys, `nextToken`/`pageToken`, ordering (newest-first where real cloud does), stable across calls.

Every gap → a `BUGS.md` entry (filed before fix) → real fix → a regression test driving the real client → surface-table/coverage-matrix note if the op status changes.

## Reference adaptors (the conformance oracle)

CI cannot reach real clouds (live-cloud is a separate gated track — BUG-1075). The real official clients encode real-cloud behaviour and ARE reachable in CI:
- AWS: `aws-sdk-go-v2`, `aws` CLI (latest botocore), terraform-provider-aws.
- GCP: cloud client libs, `gcloud`, terraform-provider-google. (Compute/Network apply is Linux-CI-only.)
- Azure: `az*` SDK, `az` CLI, terraform-provider-azurerm. (TF stack is Docker-only; in-Docker `go test -timeout` hardcoded 300s.)

When in doubt about a wire shape, verify with `--debug` / serializer source (`go list -m -f '{{.Dir}}'` then grep serializers.go/deserializers.go), not assumption.

## Stage plan

| Stage | Scope | Status |
|---|---|---|
| 1 | AWS conformance sweep + fixes + regression tests | **in progress** |
| 2 | GCP conformance sweep + fixes + regression tests | pending |
| 3 | Azure conformance sweep + fixes + regression tests | pending |
| 4 | Go type hardening across all sims (`docs/GOLANG_STRONG_TYPING.md`) | pending |
| 5 | Simulator UI hardening (aws/azure/gcp UIs) | pending |
| 6 | Wrap: coverage matrix + surface tables + continuity reconcile | pending |

Each stage ends with: `go test ./...` (affected modules) green, golangci-lint v2.10.1 clean, a commit, a push (CI run), and this doc updated.

## Progress log

### Stage 1 — AWS conformance
- **Started:** 2026-06-10.
- **Approach:** walk the 33 AWS surfaces; per surface run the round-trip / error / pagination probes above against the SDK + CLI; file+fix+regress each gap.
- **Findings:** _(none yet — sweep starting)_
- **Next:** begin the round-trip drift probe on the highest-traffic surfaces (s3, dynamodb, ec2, ecs, lambda, iam), then fan out.

### Stage 2 — GCP conformance
- Not started.

### Stage 3 — Azure conformance
- Not started.

### Stage 4 — Go type hardening
- Not started. Candidates surface during stages 1-3 (stringly-typed states, bare-ID transposition, `map[string]any` request decode). Apply typed enums/IDs/sealed sums per `docs/GOLANG_STRONG_TYPING.md`.

### Stage 5 — Simulator UI hardening
- Not started. 3 UIs (`ui/packages/simulator-{aws,gcp,azure}`, ~8 TS files each on a shared core). Tighten types, fix bugs, verify.

### Stage 6 — Wrap
- Not started. Reconcile `specs/SIM_TEST_COVERAGE_MATRIX.md` + surface tables; final continuity pass.
