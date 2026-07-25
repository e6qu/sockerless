# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Completed Baseline

`fix/shauth-product-ui-markers` qualified Sockerless through its real product interface. Admin and all three simulator dashboards exposed the authenticated operator and the real sign-out control to the browser matrix, which asserted the rendered username and clicked the product control instead of relying on a protocol-only validation page.

`fix/shauth-final-pin-ci-timeouts` had completed the exact Shauth relying-party contract for the same four applications, enforced UI-first release builds and bounded nightly fuzz concurrency, and held every ordinary GitHub Actions job to 15 minutes without dropping any of the 630 AWS CLI tests.

The real pinned Shauth, PostgreSQL, Ory Hydra, freshly compiled relying parties, and Chromium matrix passed direct and catalog entry, relying-party and provider logout, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and credential isolation.

## Remaining Work

The Google Cloud, Amazon Web Services, and Microsoft Azure consoles now read
real cloud APIs over each console's own server-side Shauth federation
(BUG-2635) *and* present their real visual language, verified against the live
reference. All three simulator consoles now read only real cloud APIs — no
`/sim/v1/*` dashboard endpoint remains on any of them. The Azure portal
exchanged the operator's Shauth assertion through Microsoft Entra Workload
Identity Federation (`client_credentials` + JWT-bearer `client_assertion`; the
simulator verifies it against the identity's federated identity credential) and
read the real Azure Resource Manager and Azure Monitor Log Analytics APIs,
distinguishing their token audiences; its Fluent-style icon set replaced the
placeholder glyphs. The method is recorded so it is not repeated from memory:
compare against the live reference, extract ground-truth tokens, vendor the
open assets (Material Symbols SVG + Roboto for GCP; Open Sans for AWS; Fluent
UI System Icons for Azure), pin structural proxies, report honestly.

The simulators now enforce credential verification on their data planes
(BUG-2625 closed): AWS verifies SigV4 signatures at the control plane and S3;
Google Cloud and Azure verify the bearer they minted (signature, issuer,
audience, expiry). Every SDK/CLI/Terraform suite, the sockerless backends, and
the relying-party provisioning authenticate for real, and the console browser
e2e — which reaches the enforcing simulator unauthenticated — moved its
data-render assertions to the authenticated relying-party path. The AWS ECS
Terraform harness terminates deterministically (BUG-2569 closed). The remaining
console fidelity work is live-cloud (BUG-1075): exercising each console's
federation and reads against the real cloud, where the proprietary fonts and
icons (Amazon Ember, Segoe UI) stay honest approximations.

BUG-2633 closed: a repository gate (`scripts/check-required-status-checks.sh`,
pre-commit + the `build-gates` CI job) now enumerates every check name the
workflows can emit and fails the pull request when a required context in
`.github/required-status-checks.txt` is no longer emittable, so a matrix job
rename can no longer silently stall the merge queue.

The skip-if-absent sweep is complete: `scripts/check-no-tool-absent-skips.sh` now
catches the `LookPath → os.Exit(0)` and print-then-skip shapes (exempting genuine
platform/capability gates), the GCP `exec.LookPath("gcloud") → os.Exit(0)` skip
and its Azure counterpart became install-or-fail-loud, and the remaining
`docker`/`session-manager-plugin`/`git`/`gcloud`/`nsenter` tool-absent skips were
removed (vestigial) or made `t.Fatal` against CI-provided tools.

Still open, all externally gated: BUG-2523 and BUG-2441 (Bleephub product/UI, in
the separate bleephub repository), BUG-1345 (AzureAD Terraform provider,
upstream-blocked), and BUG-1075 (live-cloud validation, needs real cloud
credentials).

Phase 1 of the console self-service roadmap ([PLAN.md](PLAN.md) § "Console
Self-Service") shipped: all three consoles mint real CLI credentials for
Shauth-authenticated operators (AWS IAM access keys, Google Cloud
service-account key JSON, Microsoft Entra client secrets) over federated
credentials and real cloud APIs, with CLI tests proving each minted credential
authenticates the vendor CLI and the Shauth relying-party matrix driving the
AWS and Google Cloud minting UIs (the Azure browser flow is staged into the
deployment phase as BUG-2640 — the portal's browser-side federation exchange
is same-origin-only, so the separately-deployed shape needs the server-side
broker and faithful CORS). Proving the loops also hardened the GCP token
endpoint (assertion signatures now verified against registered, revocable
public keys) and the Entra token endpoint (client secrets validated for
registered applications), and deleted the invented `/sim/v1/entra/users`
routes.

Phase 2 shipped: account and project management — the Google Cloud Resource
Manager slice (replacing a faked partial v3 surface) with the console's real
project picker, the Azure Microsoft.Subscription alias API with the portal
Subscriptions blade (its Terraform coverage as the `tf (azure subscription)`
shard), and the AWS Organizations console page — all with SDK/CLI/Terraform
coverage and the relying-party matrix driving the AWS and Google Cloud browser
flows. Phase 3 shipped: `sockerless login`/`logout` — the RFC 8252 loopback PKCE
flow against Shauth (public Hydra client, one-time consent), wiring
vendor-native credentials (AWS `web_identity_token_file` profile with
`endpoint_url`, GCP workforce `external_account` ADC activated via `gcloud
auth login --cred-file`, `az login --federated-token` against a TLS simulator
instance), with the simulator's missing STS introspection slice added
(BUG-2641) and the relying-party matrix driving the whole terminal story.

Phase 4 shipped, completing the Console Self-Service roadmap: a committed
`deploy/` recipe (Shauth stack + Admin + three simulators behind a Caddy TLS
proxy, real-API provisioning, a `make deploy-smoke` gate) and BUG-2640 — the
Azure portal now federates through the console's server-side broker with
faithful ARM/Graph CORS, the harness running the Azure console and cloud as
separate processes, which unblocked the Azure browser minting flow deferred
since phase 1. All four phases (credential minting; account/project/
subscription management; `sockerless login`; deployment + Azure federation)
have merged or are in flight.

The console/simulator fidelity follow-ups filed during the roadmap were closed (BUG-2637 AWS console table actions, BUG-2638 GCP serviceAccounts 409, and BUG-2642 — the Lambda SigV4/IAM enforcement gap found while fixing them). BUG-2639 (Azure implicit grant for unregistered client ids) is now closed: the simulator seeds a bootstrap Entra application and the implicit-client branch was deleted, so an unregistered client id returns the real AADSTS700016. It was a clean single-coordinate consolidation, not the feared mass migration.

No roadmap phase remains queued. Candidate next work: the staged live-cloud
validation backlog (BUG-1075), the deployment recipe's real-registry/GHCR
publish path, or new console surfaces as the product grows — pick with the
user. Filed follow-ups with fix
shapes: BUG-2637 (inert default AwsTable actions), BUG-2638 (GCP
`serviceAccounts.create` overwrite vs 409), BUG-2639 (Entra implicit grant for
unregistered client ids), BUG-2640 (Azure portal federation deployability).

1. The remaining Shauth catalog applications still needed the same product-interface contract before Shauth's launch-interface assertion could be enabled: SameOldChat, Intraktible, Bleephub, Bleeplab, Sharecrop, ECS Dev Desktop, and the simulator console. E6IRC already carried it.
2. Shauth still needed its strengthened validator merged last, so that qualification exercised each application's real launch interface and its registration-contract revalidation rather than the technical validation page alone.
3. The merged revision needed immutable Admin and simulator images published and deployed in the shared development environment, followed by the same exact browser matrix against the live origins.
4. The standalone Bleeplab GitLab Runner consumer needed another real run against the merged Sockerless revision so its slower two-cycle source-fetch path exercised the corrected wait-channel and CloudWatch stream generation.
5. BUG-2569 still required deterministic termination of the local Amazon Elastic Container Service Terraform simulator apply/destroy harness without changing the real provider path.
6. BUG-2625 still required provider-faithful simulator credential issuance and verification for AWS, Google Cloud, and Microsoft Azure, with exact errors and official SDK, command-line interface, and Terraform coverage.
7. The remaining live-cloud cells in BUG-1075 still required authenticated validation before being marked green.

## Durable Validation Contract

- Production builds created every frontend before any UI-bearing Go binary.
- Workflow changes kept every ordinary job at or below 15 minutes and preserved exact AWS CLI shard coverage.
- Fuzz changes exercised every discovered target and treated a missing module, build failure, target failure, or crasher as a real failure.
- Shauth changes ran the real PostgreSQL, Ory Hydra, relying-party, and Chromium matrix from the exact pinned provider revision.
