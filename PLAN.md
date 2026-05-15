# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain. Including request-body tolerances: if real GitHub accepts string-coerced booleans (what `gh api -f` sends), bleephub accepts them too. The `gh` CLI must work directly against bleephub — not via URL hackery.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites (the `gh` binary, the official `actions/runner`, real Docker SDKs, Terraform providers).
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find. **In particular: we are not in legacy maintenance — no shims for old bleephub behavior.** If real GitHub does X, bleephub does X.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.
13. **Persistence is opt-in + fail-loud.** Operator-requested persistence (`BLEEPHUB_PERSIST=true`, `SIM_PERSIST=true`) that fails to open must `log.Fatalf`. Never silently fall back to in-memory (BUG-985/986).

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #129 | 135 | Sim host model + 3-tier coverage + native arm64 CI runners. |
| #130 | 128 | Runner job timeout (bootstrap timer + cloud-native cap). |
| #131 | 124 | Network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only). |
| #132 | 125 | DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none). |
| #133 | 126 | Access driver (iam-role / id-token / mTLS / none-internal). |
| #134 | 127 | Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| #135–136 | 121b | Azure sim hardening, driver consolidation pattern B, network-discovery adapter consolidation, AZF/Lambda DNS, Azure AD access. |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, `TopologyManager`, lifecycle endpoints, UI Topology page, per-instance logs + console, cloud-resources rollup, sim UI parity, per-instance state isolation + BUG-985/986). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| #145–146 | 87 + 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger) + component-side OTel SDK wiring. |
| #147–149 | 91 + 91b + 91c | `BackingMemory` translator across 5 backends; Lambda volume_translator framework migration; cloudrun + gcf `BackingPDEphemeral` rejection. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf (closes BUG-944, ships BUG-987). |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| #153 | 153 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat (13 sub-tasks; Docker harness 50/50 PASS). |
| #154 | 154 | Broad GitHub API sweep — reactions, releases, deployments + environments, PR review comments + threads, Checks, Actions OIDC + JWKS, Pages, branch protection. |
| #155 | 155 | bleephub-specific docs refresh — `bleephub/README.md`, `docs/BLEEPHUB_GH_CLI.md`, `specs/BLEEPHUB_GITHUB_API_PARITY.md`, `ARCHITECTURE.md` block. |
| #156 | 156 | Project-wide docs refresh + bleephub Quick start + `gh` CLI `--hostname` clarification + GCP `google.golang.org/api` v0.278.0 → v0.279.0. |
| #157 | 157 | Component ⇄ reference-adaptor docs sweep (only `backends/docker` covered; remaining components queued as Track A in DO_NEXT.md). Experimental/security caveat on root README. BUG-991 surfaced + staged. |
| #158 | 158 | BUG-991 + BUG-992 fixes; `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md` 15-approach research-only catalogue; 3 project-local Claude skills under `.claude/skills/`. |

## Active + planned phases

Each entry: scope, why, acceptance. Pick from [DO_NEXT.md](DO_NEXT.md).

### Phase 159 — AWS simulator: CloudFront + Amplify + IAM/Route 53/WAFv2/ACM (complete on PR #159, awaiting merge)

Expand `simulators/aws/` to cover the front-of-house CDN + website-hosting surface most AWS Terraform stacks reach into. **All 11 sub-tasks shipped on `phase-159-aws-sim-cloudfront-amplify`** (P159.0 dep tidy → P159.10 docs + end-to-end stack test). CI green on every push; awaiting user merge.

Today's sim handles ECS / ECR / IAM / EC2 / EFS / Lambda / KMS / SSM / S3 / STS / SecretsManager / DynamoDB / CloudWatch / CloudMap / Lambda Runtime / metadata. Phase 159 adds:

| Service | Wire | Why now |
|---|---|---|
| CloudFront | REST + XML | The CDN front-end for Amplify, S3 static sites, and most Terraform-stamped production stacks. |
| AWS Amplify | JSON | Website hosting + branch-per-env deployments; depends on CloudFront under the hood. |
| WAFv2 | JSON (scope-aware) | Web ACL associations on CloudFront distributions are the standard prod-shape. |
| ACM | JSON (us-east-1 pin) | Cert issuance + DescribeCertificate for CloudFront distribution aliases. |
| Route 53 (ALIAS records) | REST + XML | ALIAS A/AAAA records targeting CloudFront distribution DNS names; the production way to wire a custom domain. |
| IAM additions | JSON (existing handler extended) | Service-linked roles `AWSServiceRoleForCloudFrontLogger` + `AWSServiceRoleForAmplify`; OIDC providers for Amplify SSR. |

Reference adaptors per Phase 157 frame:

- **`aws cloudfront` CLI** — `create-distribution`, `get-distribution`, `update-distribution`, `delete-distribution`, `list-distributions`, `create-invalidation`, `create-function`, `publish-function`, `create-origin-access-control`, `associate-alias`, tag CRUD.
- **`aws amplify` CLI** — `create-app`, `update-app`, `delete-app`, `list-apps`, `create-branch`, `start-deployment`, `start-job`, `create-domain-association`, `create-webhook`.
- **`aws wafv2` CLI** — `create-web-acl --scope CLOUDFRONT`, `associate-web-acl`, `list-web-acls`, `update-web-acl`, `delete-web-acl`.
- **`aws acm` CLI** — `request-certificate`, `describe-certificate`, `delete-certificate`, `list-certificates`, `add-tags-to-certificate`.
- **`aws route53` CLI** — `change-resource-record-sets` with `AliasTarget`, `list-resource-record-sets`.
- **AWS Go SDK** — `aws-sdk-go-v2/service/{cloudfront,amplify,wafv2,acm,route53}`.
- **Terraform `aws` provider resources** — `aws_cloudfront_distribution`, `aws_cloudfront_function`, `aws_cloudfront_origin_access_control`, `aws_amplify_app`, `aws_amplify_branch`, `aws_amplify_domain_association`, `aws_amplify_webhook`, `aws_wafv2_web_acl`, `aws_wafv2_web_acl_association`, `aws_acm_certificate`, `aws_route53_record` (with `alias{…}`).

Wire-protocol notes (load-bearing):

- **CloudFront speaks XML.** Path style is `/2020-05-31/distribution/{id}`; bodies are `<Distribution>…</Distribution>`. Existing sim handlers are JSON-only; need an XML encoder/decoder shape next to the JSON one. Match `aws-sdk-go-v2/service/cloudfront`'s exact element order — the SDK rejects out-of-order XML on some types.
- **Route 53 speaks XML.** Same family.
- **WAFv2 scope dimension** — `CLOUDFRONT` scope is global (us-east-1); `REGIONAL` is per-region (ALB/API Gateway). Phase 159 covers CLOUDFRONT scope; REGIONAL can come later if a backend needs it.
- **ACM us-east-1 pin** — CloudFront only accepts certs from `us-east-1`. Sim must enforce this on `ViewerCertificate.ACMCertificateArn` references and reject mismatches with the same error shape the real AWS API uses.

Acceptance:

- Per-service `simulators/aws/{cloudfront,amplify,wafv2,acm,route53}.go` handler files matching the existing per-service file pattern.
- `simulators/aws/sdk-tests/` adds test functions per service driving the real AWS Go SDK against the running simulator.
- `simulators/aws/terraform-tests/` adds Terraform plans per service using the real Terraform `aws` provider with `endpoints {}` block overriding to the sim.
- `simulators/aws/cli-tests/` adds `aws` CLI smoke per service.
- `simulators/aws/API_SPEC.md` updated to enumerate the new verbs covered + last-green count.
- `simulators/aws/README.md` updated in Phase 157 adaptor-led shape (currently part of Track A; can be folded in here).

Sub-task breakdown + commit layout in [DO_NEXT.md § Phase 159](DO_NEXT.md). Each sub-task = one commit; CI runs per commit. Phase may span multiple sessions; state save discipline (per `.claude/skills/avoid-vibe-slop`) preserves continuity.

Out of scope:

- WAFv2 REGIONAL scope (deferred until an ALB/API Gateway need surfaces).
- CloudFront edge functions actually executing JavaScript (return success; do not interpret the code).
- Lambda@Edge association beyond the metadata layer.
- Amplify build orchestration actually running the build (`StartJob` returns success + a synthesised job ID; no real npm install).
- ACM certificate DNS-validation polling loops (`pendingValidation` → `issued` transition can be eager).
- Service-linked role *enforcement* (sim accepts requests without verifying the SLR exists; create the records on demand).

### Phase 157 — Component ⇄ reference-adaptor docs sweep (partial; Track A in DO_NEXT.md)

PR #157 covered `backends/docker` only. The remaining components (backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}, simulators/{aws,gcp,azure}, simulators/README.md showcase, cmd/sockerless, cmd/sockerless-admin) carry forward as Track A. Note: the `simulators/aws/README.md` portion likely lands inside Phase 159 since the new services are being added there.

Doc shape (locked-in from #157): lead with adaptor, then validation, wiring, sample (real captured output), out-of-scope.

### Phase 153–156 — Closed

bleephub ↔ GitHub API parity (153) + broad GitHub API sweep (154) + bleephub docs (155) + project-wide docs (156). Headlines in the PR index above; narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); per-bug detail in [BUGS.md](BUGS.md). Spec at [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

### Phase 158 — Closed (PR #158)

BUG-991 + BUG-992 fixes (handler→`s.self` delegation closed two fallback-hiding-bugs); `docs/VIBE_CODING.md` 23-pattern catalogue; `docs/GOLANG_STRONG_TYPING.md` 15-approach research-only catalogue; 3 project-local Claude skills under `.claude/skills/`. Narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

### Live-cloud validation track

Per-backend live-cloud sweeps separate from unit/sim CI. Live-AWS ECS validated 2026-04-20. Outstanding:

- Lambda live (deferred from Phase 86).
- Cloud Run Services / ACA Apps live (closed in code 2026-04-21 behind `UseService` / `UseApp`).
- AZF + cloud-dns on Azure live (new in #136).
- Lambda + service-mesh on AWS live (new in #136).
- ACA / AZF + Azure AD access on Azure live (new in #136).

One branch per cell; teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Phase 91d — Real pd-ephemeral on cloudrun + gcf

**Bookmarked indefinitely.** Cloud Run's `runpb.Volume` lacks a PD field; Admin API doesn't expose PD attach as a first-class primitive. Real implementation requires either a sockerless GCE-style backend or a Cloud Run feature change. Reject-with-pointers shape (Phase 91c, PR #149) stays in place.

## Driver phase template

Storage backing (Phase 127) is the pilot. Each driver phase follows:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impl (pattern B: shared by both backends in that cloud).
4. `backends/<cloud-product>/server.go` — wires the per-cloud driver into the backend's registry at startup.
5. Operator config: env var selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Sockerless GCE-style backend (would unlock Phase 91d real `pd-ephemeral` for real workloads).
- Marketplace / billing on bleephub (currently out of scope — most apps don't use them; revisit if a real consumer asks).
