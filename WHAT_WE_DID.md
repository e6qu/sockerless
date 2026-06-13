# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file kept the recent chain and a compact foundation summary.

## 2026-06-14 - Faithful build→push→pull for gcp Cloud Build (BUG-1785, gcp half — closes the bug)

The gcp half mirrors the azure ACR Tasks fix below and completes BUG-1785.
The gcp Cloud Build sim built the overlay into the host's local docker
daemon and the Cloud Run / GCF workload ran that local copy — the sim's
registry never reflected the build, a non-faithful shortcut.

- The Cloud Build `push` step (`simulators/gcp/cloudbuild.go`) now does a
  real `docker push <ref>` + `docker rmi`, exactly as real Cloud Build with
  `IsPushEnabled`. The registry, not the build host, holds the image; the
  Cloud Run / GCF workload pulls it over the standard `/v2/` API.
- The overlay registry host is a **coordinate**: `gcpcommon.OverlayRegistryHost`
  reads `SOCKERLESS_GCP_AR_ENDPOINT` (default = the real
  `<region>-docker.pkg.dev`), parallel to `SOCKERLESS_AZURE_ACR_ENDPOINT`.
  The backend builds the *real* registry ref for cloud and sim alike.
- The cloudrun + cloudrun-functions integration harnesses set that coordinate
  **per-target, exactly like `endpointURL`** — to the sim's published `/v2/`
  at `127.0.0.1:<port>` (Docker auto-trusts loopback as insecure). There is
  **no `if sim` / `if target == "sim"` branch** in backend or test code: a
  sim run differs from a cloud run only in coordinates, so the client path
  is identical and the test proves the real path, not a sim-special one.
- `TestCloudBuild_FaithfulBuildPush` asserts the built image lands in `/v2/`
  and is gone from the local daemon. The `test (gcp backends)` and gcp/gcf
  faas-smoke CI jobs (which always build overlays) exercise the full
  build→push→pull round-trip against the sim.

This is the same lesson as the azure half, generalized into a rule: the
coordinate-only pattern is now documented in
[specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md)
§ "Faithful build → push → pull" and [AGENTS.md](AGENTS.md) § "A sim test
differs from a cloud test ONLY in coordinates", cross-linked both ways.

## 2026-06-13 - Faithful build→push→pull for ACR Tasks (BUG-1785, azure half)

The ACR Tasks sim built the overlay into the host's local docker daemon and
the ACA App ran that local copy — so the sim's registry never reflected the
build, and the run used a non-faithful shortcut. The user flagged it: the
sim must not rely on functionality that isn't strictly faithful to the
cloud. A first attempt to close it went wrong — it coupled the shared
workload runner directly to the registry's in-process store, a dependency
that doesn't exist in the cloud (compute pulls from the registry over the
public `/v2/` API like any client) — and was reverted.

The faithful fix keeps the sim's services agnostic:

- The ACR Tasks build now does a real `docker build` + `docker push` to the
  registry + `docker rmi`, exactly as real ACR Tasks (IsPushEnabled). The
  registry, not the build host, holds the image.
- The ACA App run pulls it over the standard registry API (the existing
  `StartContainerSync` pull path). Registry and compute talk only through
  `/v2/` — no in-process coupling.
- The backend honors a configurable ACR registry endpoint
  (`SOCKERLESS_AZURE_ACR_ENDPOINT`, a legit sovereign/custom-cloud override)
  so the harness can point the overlay ref at a reachable, auto-insecure
  endpoint.
- The harness publishes the sim `/v2/` at `127.0.0.1:5000` and (only on a
  podman machine, which unlike Docker doesn't auto-trust loopback
  registries) drops a scoped, idempotent insecure-registry entry. On Docker
  and Linux CI it's a no-op.

Validated end-to-end: ACA harness TEST 12 (container-mode job) passes with
the real build→push→pull, and the ACR Tasks SDK test asserts the built
image lands in a real registry (a throwaway `registry:2` stand-in) and is
gone from the local daemon. The gcp Cloud Build half of BUG-1785 — same
pattern, but it must thread the cloudrun/gcf overlay flows and their
integration tests — remains as a separate, larger change.

## 2026-06-13 - ACA GitHub container-job topology: TEST 12 green (BUG-1782 + BUG-1783)

Got the GitHub container-mode job (TEST 12) passing on the **ACA** backend
through the bleephub official-runner harness — the first container backend
beyond ECS to run a container job end-to-end, and the validation that the
whole ACA App-overlay + reverse-agent path works.

Two backend/harness fixes made it work, plus the `provision_aca` wiring:

- **BUG-1782:** `NewACRBuildService` (backends/azure-common) ignored
  `SOCKERLESS_ENDPOINT_URL` — its ARM + azblob clients targeted real Azure,
  so the App-overlay bootstrap-build path couldn't reach the sim. Now it
  threads the endpoint: ARM `RegistriesClient`/`RunsClient` use the
  `cloud.Configuration` override (+ `InsecureAllowCredentialWithHTTP`), and
  the blob client is resolved lazily from the storage account's advertised
  `primaryEndpoints.blob` (via `armstorage` GetProperties) — the faithful
  way to reach storage on a custom/sovereign/simulated cloud. Both
  `aca`/`azf` call sites pass `config.EndpointURL`.
- **BUG-1783:** the bleephub `Dockerfile` built `sockerless-cloudrun-bootstrap`
  + `sockerless-agent` glibc-dynamic (CGO on by default under `golang:1.25`).
  Baked into an alpine overlay they failed to exec with `No such file or
  directory` (missing dynamic loader), so the bootstrap never dialed back
  and ACA exec timed out. The canonical agent Makefile already uses
  `CGO_ENABLED=0`; the harness Dockerfile was the anomaly — now static too.

`provision_aca` now drives the App-overlay path: `SOCKERLESS_ACA_USE_APP=1`
+ ACR + a build-context blob container + an arch-matched build platform,
and pins a deterministic `<account>.blob.localhost` storage endpoint
(`SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` + an `/etc/hosts` alias,
since `*.localhost` isn't special-cased by the container resolver). The
full chain — sim ACR Tasks builds the overlay → ACA App runs it → static
bootstrap dials back → `docker exec` job steps — runs green.

TEST 13 (service container) is the next hurdle: a sibling service App's
alias doesn't resolve from inside the job App (filed **BUG-1784**); TEST 14
(dispatcher) needs a published-port wiring fix. Then Cloud Run + GCF.

## 2026-06-13 - Azure sim ACR Tasks slice (overlay-build keystone for ACA/AZF)

Added a faithful **ACR Tasks quick-build** slice to the azure simulator —
`POST .../registries/{name}/scheduleRun` (DockerBuildRequest LRO) + `GET
.../runs/{runId}`. This is the cloud-API the Azure backends call to build
their reverse-agent bootstrap **overlay image**: `backends/aca` and
`backends/azure-functions` issue `RegistriesClient.BeginScheduleRun` with a
`DockerBuildRequest` and poll the run. The handler fetches the build
context from the sim's blob storage (where the backend's azblob upload
landed), runs `docker build` on the host engine — the sim's build
infrastructure, exactly as the GCP Cloud Build slice
(`simulators/gcp/cloudbuild.go`) does — and tags the image into the local
daemon, where `StartContainerSync` resolves it by tag without a registry
pull. The run completes synchronously and is returned as a 200 with a
terminal-state `Run` body, which the azure-core LRO poller resolves via its
NopPoller path (a 202 would be a hard error for a POST LRO). No
sockerless-aware special-casing: any ACR Tasks client reaching `scheduleRun`
gets the same behavior. SDK tests (`acr_tasks_test.go`) exercise the full
path — upload context, BeginScheduleRun, assert the image is really present
in the local daemon, GetRun round-trip, and a missing-context build that
reports the Run as `Failed`.

Standing up the ACA topology cell on this slice surfaced **BUG-1782**:
`NewACRBuildService` (backends/azure-common) ignores `SOCKERLESS_ENDPOINT_URL`
— it builds the ARM + azblob clients against real Azure — so the App-overlay
path can't reach the sim (or any custom cloud) yet. Filed; the fix (thread
the endpoint override + target the account's advertised blob endpoint) plus
the harness wiring (UseApp, ACR, build-context container) and the
reverse-agent exec validation are the next steps of the Arc-2 ACA build.

## 2026-06-13 - All-backend metadata network driver + experiential-parity principles (Arc 2 groundwork)

Stand-up of the ACA cell of the bleephub GitHub topology harness (Arc 2)
surfaced and fixed a cross-backend defect, and the work crystallized the
principles that govern the rest of the pod + runner sweep. The harness
plumbing itself (multi-backend image, `BLEEPHUB_BACKEND` parameterization)
lands with the ACA cell in the following arc, once container-job exec is
assembled through faithful cloud APIs.

- **BUG-1780:** only the ecs backend overrode `Drivers.Network` to the
  metadata-only `SyntheticNetworkDriver`; lambda/cloudrun/gcf/aca/azf fell
  through to `BaseServer.InitDrivers`' real-Linux-netns driver (`ip netns
  add` + veth). That 400s a `docker network create github_network_<hex>`
  on a host without iproute2 and leaks a meaningless kernel netns where it
  succeeds. Docker networks on these backends map to *cloud* primitives
  (Lambda VPC ENIs, Cloud Run/GCF VPC-connector + Cloud DNS, ACA + AZF NSG
  / Private DNS), never a local netns — so all five now mirror ecs. All
  six cloud backends' test suites pass; a harness run confirmed ACA's
  `docker network create`/`delete` now provisions + tears down the NSG +
  Private DNS zone cleanly.

Codified the **experiential-parity principle** the user articulated:
sockerless backends are providers of the Docker+Podman REST API assembled
out of cloud primitives, and the goal is that a user's experience with
containers, pods, networks, and volumes inside any backend is the same as
local Docker/Podman. Every Docker abstraction is *composed* from cloud
primitives on every backend, FaaS included — including localhost /
shared-loopback networking between pod members — and a FaaS platform that
can't run multiple containers per function is our job to assemble (native
sidecars where offered, else a pod from multiple functions wired by cloud
DNS + a shared volume, with the agent proxying localhost to siblings), not
to reject. And the **sims stay faithful cloud slices** — no special / fake
functionality layered on to support sockerless backends or runners; a
backend's need is met by implementing the real cloud API, never a sim-side
hook. Written into AGENTS.md (new "Assemble Docker abstractions" section +
the cloud-API-fidelity rules-out list), CLOUD_RESOURCE_MAPPING.md
(universal rule 8), and the AZF README; filed **BUG-1781** and staged the
FaaS multi-container assembly as PLAN § Next #1 (replacing the interim
fail-fast rejections).

## 2026-06-13 - Pod-model correctness across backends (Arc 1 of the pod + runner focus)

Opened a sustained focus on the pod model and GitHub/GitLab runner
integration across all backends. Built a verified gap matrix first
(correcting the recon agents' over-claims): only Lambda is live-proven
(BUG-1075); the GitHub container-job topology is sim-proven for ECS only
(the bleephub harness); the other backends have per-backend GitLab
stdin-attach unit tests but no full-topology proof; AZF can't run
multi-container pods. Arc 1 fixed the verified pod-model bugs:

- BUG-1778: Lambda delegated all four Pod lifecycle methods, and GCF
  delegated Stop/Kill/Remove, to BaseServer.Pod* — which read
  Store.Containers and call Store.ForceStopContainer (local in-memory,
  no cloud call), so `docker pod stop/kill/rm` (and lambda `pod start`)
  left the underlying Lambda function / Cloud Run Service running.
  ECS/Cloud Run/ACA already override these to loop their cloud-aware
  Container* methods; lambda+gcf now mirror that. The isolation lint
  forbade BaseServer.Container* but not BaseServer.Pod{Start,Stop,Kill,
  Remove} — that gap is closed so the leak class can't recur.
- BUG-1779: AZF's single-invocation multi-container rejection fired
  late (only at the 2nd ContainerStart, after create succeeded);
  PodStart now rejects up front with one clear error, and the
  constraint is documented in the azf README (rules out `services:` and
  sidecar `container:` jobs on AZF).

Verified non-issue (left as-is): GCF injects pod host-aliases on the
main container only — correct, because sidecars are raw user images
that don't run the sockerless bootstrap, so only main can write
/etc/hosts; main→sidecar (the services-contract direction) works and
sidecar→sidecar-by-name isn't achievable or needed.

## 2026-06-13 - Pod-model / lifecycle review fixes (bundled into the audit PR)

Reviewed the pod + container lifecycle across all 7 backends for needless
delays, fixed timeouts, polling inefficiency, and constraint mismatches.
Verified each high-severity claim against source (the review agents
overstated several, as on the sim audit). Two genuine weaknesses fixed:

- BUG-1776: ACA `attachStream.Read` did a bare `<-respReady` with no
  deadline — the AZF twin bounds the identical buffered-attach wait with a
  deadline (the BUG-1505 fix) but it was never ported. A stalled or
  lifetime-capped ACA Job would strand an attached docker/StdCopy reader
  forever. Ported the AZF pattern (bootstrap window + job-run budget).
- BUG-1777: the recovery-path `WaitForExit` (cloudrun + aca) re-ran a full
  ListJobs+ListServices (with per-pod GetService follow-ups) on every tick
  to check one container's exit. Narrowed to resolve the backing Job once
  then poll that single job's state — the identical resolveExecutionState /
  resolveJobState derivation, one resource — with a list fallback for
  Service/App-backed and vanished jobs. Cloud Run's `waitForServiceURL`
  flat-2s/150-call loop got exponential backoff (1s→15s). GCF left alone
  (its hot path is already event-driven; FaaS has no persistent exit
  state to narrow to).

Deliberately NOT changed (verified non-issues): pod deferred-start has no
timeout but doesn't block a goroutine and mirrors docker-compose semantics;
the 30s stdin-capture waits are warn-and-proceed generous upper bounds;
the O(n) queryTasks/queryFunctions is the documented stateless-invariant
cost; the ACA app-readiness "returns before replica ready" is masked by the
reverse-agent wait in the common path and the multi-container case is
intra-revision (no cross-app DNS) — adding an unconditional revision-ready
poll would regress hot-path latency for an unproven race. The forced delays
(Fargate task-start, Lambda/GCF/AZF limits, ARM LROs) are genuinely
cloud-imposed. The hot-path exit detection (pollTaskExit/pollExecutionExit)
is already single-resource + event-driven; pod materialization already
builds member overlays in parallel and deploys atomically.

## 2026-06-13 - Sim fidelity audit pass (probe the load-bearing gaps)

Ran a registered-op-vs-test-coverage sweep across all three sims, then
applied the established discipline — "untested ≠ working; probe with
assertions" — to the gaps that are both load-bearing (a backend or
terraform actually calls them) AND complex enough to harbor a real
bug. The broad coverage maps were noisy (most "untested" ops are
out-of-slice or already had dedicated fidelity tests the op-name grep
missed — Cloud Map, EFS, ECS are well-covered), so the value was in
narrowing to what sockerless depends on. Three real fidelity bugs,
each confirmed by a real-SDK probe mirroring the exact backend call
pattern, each fixed with a permanent regression test:

- BUG-1773: AWS CreateSecurityGroup never rejected a duplicate name in
  the same VPC — the ECS per-job-network create
  (backends/ecs/network_cloud.go) relies on InvalidGroup.Duplicate to
  reuse an existing SG by name+VPC; against the sim a retry silently
  minted a second SG. Now rejects same name+VPC (different VPCs still
  reuse a name).
- BUG-1774: AWS AuthorizeSecurityGroup{Ingress,Egress} never rejected a
  duplicate rule — the backend re-applies its self-referencing ingress
  rule and swallows exactly InvalidPermission.Duplicate; the sim
  appended a second identical permission, so DescribeSecurityGroups
  read-back accumulated duplicates. Now detects an equivalent existing
  permission (protocol + ports + shared target) and 400s it.
- BUG-1775: GCP rrsets.list ignored its name/type query filter — the
  Cloud Run service-discovery path uses .Name(fqdn).Type("A"); the sim
  returned the whole zone. The backend re-filters client-side so it
  wasn't broken, but the sim diverged from real Cloud DNS. Now honors
  the filter.

## 2026-06-12 - Runner-as-cloud-task topology, sim-proven (cells 1+2 minus the live pass)

The bleephub official-runner harness became the topology proof. Its
image now bundles simulator-aws + sockerless-backend-ecs + the
dispatcher next to bleephub and the runner; the make target mounts the
host docker.sock and a sim-EFS host dir at an identical path inside
and out, so the runner's workspace IS an EFS access point and
`container:` jobs dispatched through the backend land as sim-ECS tasks
on the host engine sharing it. TEST 12 asserts the contract from the
outside: a file written inside the job container shows up on the
runner's EFS workspace. TEST 13 runs a `services:` nginx reachable by
alias from the job container. TEST 14 closes the control plane: the
github-runner dispatcher (new `--api-base`; capability-probe token
verification for header-less tokens) polls bleephub for a queued job
no resident runner can take, spawns an ephemeral runner on the host
engine, and the job completes on it — 14/14 green locally
(BLEEPHUB_TEST_FROM + BLEEPHUB_HOLD knobs make single-test iteration
cheap). Every wall on the way was a real bug, filed + fixed
(BUG-1763..1771): the runner deserializes jobServiceContainers /
object-form `container:` as TemplateTokens (plain JSON maps fail job
start; `env` not `environment`); registration must round-trip
`ephemeral` or config.sh aborts, and completed ephemeral agents now
deregister; runs with no started job reported `in_progress`, hiding
them from `?status=queued` pollers; job messages baked the submitter's
request-Host as the server URL so off-host runners could run but never
complete jobs — `BLEEPHUB_EXTERNAL_URL` is the GHES-shaped fix; the
admin token advertised a scope header without `workflow`; and
dispatcher spawns add `host-gateway` only on engines that need and
accept it. Catalog: docs/RUNNERS.md D-8..D-12.

## 2026-06-12 - GitHub-runner dispatcher hardening (ARC-without-k8s parity)

Source audit of `github-runner-dispatcher-{aws,gcp,azure}` against their
contract (poll → mint token → spawn ephemeral runner → GC from cloud
state) filed and fixed BUG-1752..1762 in one PR. The P1: the Azure
sweep keyed "done" off ACA Job `ProvisioningState`, which reads
`Succeeded` the moment the resource is created — every runner Job
became reap-eligible on the next 2-min tick and `BeginDelete` kills the
in-flight execution. It now classifies the latest JobExecution's
status, the same fix the GCP spawner already carried for Cloud Run's
`TerminalCondition` (whose stale spec rows also got corrected). Both
cloud loops gained the GitHub-side offline-runner reap they claimed in
their flag help but never did, plus a 15-min orphan grace for Jobs
whose start call failed. The runner images' "60-s idle timeout" was
fiction — absent in vanilla/ecs/lambda, and a job-killing
whole-process `timeout` in cloudrun/gcf; all six entrypoints now share
a pre-pickup idle gate (watch /proc for `Runner.Worker`, exit 0 on
idle, never bound a running job). The dispatcher↔image env contract
unified on `RUNNER_REG_TOKEN`/`RUNNER_REPO` (the ECS/Lambda images
required `RUNNER_TOKEN`/`RUNNER_REPO_URL` and could not be spawned by
the dispatcher at all). New per-label knobs on all three:
`runner_job_timeout` (Cloud Run task timeout / ACA ReplicaTimeout /
docker-shape sweep enforcement) and `max_concurrent` (ARC's
`maxRunners` analog). Registration tokens left plain control-plane env
for Secret Manager (TTL-bounded, reaped with the Job) on GCP and Job
secrets + `secretRef` on ACA. Azure also got the GCP deployable
hardening (healthz/$PORT, $REPO, verify retry, rate-limit-aware loop),
2cpu/4Gi runner resources, and its first tests; `ListRunners` now
paginates. Catalog: docs/RUNNERS.md § dispatcher hurdles D-1..D-7.

## 2026-06-12 - Actions follow-ups + the bind-translation gate's mechanism

Same-day follow-up PR to #549 (BUG-1745..1750). **Cancellation is real
now**: cancelling a run sends `JobCancellation` over the runner's open
mid-job poll (the channel the pull-only broker kept exactly for this),
purges undelivered job messages, leaves `always()`/`cancelled()` jobs
runnable (they dispatch with cancelled()==true), and the run concludes
`cancelled` — proven live by the harness cancelling a `sleep 300` on the
official runner and watching the always() cleanup execute. That e2e also
caught the runner's US-spelled `Canceled` result leaking through
normalization unmapped. **Self-hosted actions work**: `uses:` resolves
from bleephub-hosted repos first (GitHub-layout tarballs built from git
storage), proven by a composite-action harness test — 11/11 official-
runner integration tests. Org **runner groups** (CRUD/membership/repo
visibility, undeletable Default) and **startup_failure run shells** for
matched-but-unstartable workflows round out the bleephub side.

On the backends, the **bind-mount→shared-volume translation** — the
documented gate for running GitHub runners as cloud tasks — reached
parity across all six container backends: lambda's config got actually
wired into its bind path, ACA/AZF gained the whole mechanism, and the
sharing contract (writer via named volume, reader via translated host
bind) is integration-proven on ECS and ACA. What remains for the
runner-on-cloud cells is topology (runner images mounting the shared
volume), not translation. Ledger: 1750 filed / 1708 fixed / 2 open.

## 2026-06-12 - Complete GitHub Actions support in bleephub

**The workflow engine now implements GitHub's server-side semantics.**
`on:` triggers parse fully (branch/tag/path filter patterns with ordered
`!` negation, real git diffs for path filters, activity types with
per-event defaults — pushes to an open PR's head branch fire
`pull_request synchronize`); `on: schedule` crons fire from a
minute-aligned dispatcher (POSIX 5-field parser, dom/dow OR rule);
reusable workflows expand server-side (synthetic gate/collector nodes,
typed+defaulted inputs, secrets inherit/mapping, outputs onto
`needs.<caller>`, 4-level nesting bound); and a real expression engine
(GitHub grammar, loose equality, full `github`/`needs`/`vars`/`inputs`/
`matrix` contexts, contains/startsWith/format/join/toJSON/fromJSON)
evaluates job `if:` and `${{ }}` templates — invalid expressions fail the
job like real GitHub.

**Secrets and variables exist at all three scopes** (repo/org/environment)
with the REAL wire contract — `gh secret set` fetches the public key,
seals with libsodium, and the server decrypts; plaintext PUTs are
rejected (the old shape no real client could ever have used). Org
visibility (all/private/selected), name rules, and org→repo→env
precedence merge into runner job messages with masks.

**Workflow runs are now first-class GitHub citizens**: every job mirrors
to a check run under a github-actions check suite; workflow_run /
workflow_job / check_run / check_suite webhook events fire at the real
emission points; PR `mergeable_state` reflects the head commit's checks
and the merge API 405s while required status checks (branch protection)
aren't green. The jobs API serves REAL per-step status/timing — the
runner's timeline records were being silently dropped because the
official runner wraps them in `VssJsonCollectionWrapper` (found against
actions/runner source; same wrapper bug fixed for console-line feeds).
Job logs persist (4MiB cap with explicit markers), run-log zips match
GitHub's layout, runs-on labels route jobs only to matching runners
(hosted aliases run anywhere), org-scoped runner endpoints exist with an
honest `busy`, reruns keep the run id and bump `run_attempt` (archived
attempts retrievable; rerun-failed-jobs carries successful results over),
and workflows enable/disable (disabled = no triggers, dispatch 403).

**The UI got the full GitHub-style Actions experience**: per-repo Actions
tab (runs list, filters, dispatch form built from parsed workflow inputs,
enable/disable), run detail (job sidebar, per-step status, live-tail
logs, rerun/cancel, artifacts, deployment approvals), secrets+variables
management with real in-browser sealed-box encryption, PR merge-box
checks section, runners page with labels/busy. Playwright 21/21, vitest
green, knip/jscpd clean.

Validation: gh Docker harness 115 PASS / 0 FAIL (now covering secrets/
variables/enable-disable/checks); the official-runner integration
harness was found bitrotted (launched binaries retired long ago —
BUG-1739), rewired to host-mode jobs (`jobContainer: null`, real
GitHub's no-container shape) and promoted to CI as
`sim (bleephub actions/runner)` — ALL 9 e2e tests green. Running the
REAL runner exposed two more latent protocol bugs, both fixed: the
broker pushed jobs at busy runners, which the runner silently drops
(BUG-1740 — delivery is now strictly pull-on-poll by free,
label-matching runners), and step `${{ }}` templates went out as
literal tokens the runner never evaluated (BUG-1741 — now
BasicExpression/format() tokens; secrets also ride message.Variables,
where the runner's ToSecretsContext actually reads them). Same PR also
closed consumer issues #548/#547: the azure-sim Entra token endpoint
accepts client_secret_basic and `/authorize` binds login_hint-resolved
users into auth codes (BUG-1742/1743). Ledger at 1743 filed / 1701
fixed / 2 open.

## Earlier milestones (compressed)

Full detail in the PR descriptions and `git log`; one line each:

- **Amplify full slice + bleephub Apps/orgs hardening** (#546) — complete Amplify control+data plane; GitHub Apps install/JWT/OAuth + org provisioning fidelity.
- **Launch hygiene** — spec-validation armed across all test surfaces incl. AWS XML protocols; azurestack retired; docs truth pass.
- **Simulator shape-drift burn-down** — all 28 allowlisted wire-shape bugs fixed (BUG-1658..1685); aws/azure allowlists emptied; knative moved to real `serving.knative.dev` paths; postgres-flexible speaks the real 202+Azure-AsyncOperation LRO.
- **Spec-based simulator validation** — vendored machine-readable cloud-API specs (`specs/cloud-api/`) + two CI gates: static surface conformance (every registered op exists in the spec) and runtime wire-shape validation (`SOCKERLESS_SPEC_VALIDATE`, ratcheted).
- **Simulator conformance + hardening (AWS/GCP/Azure)** (#537/#538/#539) — round-trip/error/pagination fidelity sweep + Go type hardening + sim-UI hardening + CI sim-module unit tests.
- **bleephub parity + durability + GitHub-style UI** (#534..#536) — SQLite/PostgreSQL persistence, filesystem + S3/MinIO git content storage, git HTTP auth, the UI restyle, and a GitHub-API fidelity sweep.
- **Cloud service-slice expansion** — AWS (Step Functions, Batch, CodeBuild, Glue), GCP (Spanner, Dataflow, Bigtable), Azure (Logic Apps, ACI), each with SDK + CLI + Terraform coverage.
- **Terraform idempotency drift sweep** (#491) — `terraform plan -detailed-exitcode` drift assertions on the gcp+azure apply stacks; ~18 read-back fidelity bugs fixed to make `tf (gcp)`/`tf (azure)` green.

## Foundations

Sockerless now includes:

- Docker API-compatible backends for local Docker passthrough and cloud-backed container/FaaS targets.
- High-fidelity AWS, GCP, and Azure local simulators, one binary per cloud, with official SDK/CLI/Terraform coverage tracked in [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).
- A real-execution substrate in `simulators/realexec`: network namespaces, bridges/veth/TAP NICs, IPAM, nftables, Firecracker VM lifecycle, health probes, and load-balancer proxying.
- Cross-cloud OCI `/v2/` registry data-plane implementations for ECR, Artifact Registry, and ACR.
- Bleephub, a GitHub Enterprise-style API simulator covering repos, issues, PRs, Actions, runners, apps, OAuth/OIDC, webhooks, packages, and admin org provisioning.
- Local HTTPS gateway infrastructure through Caddy for providers that require HTTPS endpoint discovery.

Older detailed entries were intentionally compressed out of this file. Use PR descriptions and `git log --oneline --decorate --all` for older implementation history.
