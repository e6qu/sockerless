# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact history.

## One implementation per Docker behaviour across the six cloud backends

The six cloud backends and the three per-cloud common modules had grown by
copying: about 2,100 lines of function bodies were byte-identical once the
cloud's name was normalised, and roughly as many again were near-copies that
differed only in an error message or a field name. The comment beside one of
them said why — "kept as a per-cloud copy under the no-cross-backend-import
rule" — and that rule is right, so the copies moved down instead of
sideways. Cloud-neutral behaviour now lives in `backends/core`: shared-volume
configuration (`SharedVolumes` over a per-cloud `SharedVolumeFormat`, bind
translation with a per-backend `HostBindPolicy`, the environment readers),
buffered stdin and attach (`StdinPipe`, `BufferedAttachStream`), the
bootstrap overlay image (renderer, content tag, tar context, runtime user
environment), pod manifests, network-pod materialisation, the pod lifecycle
loops, `RestartCloudContainer`, the reverse-agent export/pause/unpause
helpers, managed-volume shaping and the `ProvisionCache` every storage
manager runs on, the DNS-zone discovery skeleton, cloud-error mapping, image
reference splitting, and the embedded-UI registration. The exec envelope the
four FaaS backends post and the four bootstraps parse — which had been
declared nine times — is `agent/envelope`, imported by both sides. What is
shared within one cloud family moved to its `*-common` module: AWS SDK
configuration, ECR pull-through routing, and the EFS volume shaping; the
Cloud Run volume mapper, gcs-sync exec staging, the Cloud Logging line
reader, and the URL host helper; the Azure Log Analytics reader and the tag
flattening. The Cloud DNS and Private DNS discovery drivers became record
operations behind one `core.DNSZoneDiscovery`. Each backend keeps the
explicit `api.Backend` method the compiler demands; its body calls the one
implementation.

Comparing the copies exposed two divergences, both fixed by the merge. Azure
Container Apps and Azure Functions bounded a buffered attach read with the
bootstrap window plus the run budget; Cloud Run and Cloud Run Functions
blocked on a bare channel receive, so a stalled invocation stranded a
`docker attach` or gitlab-runner reader forever (BUG-2948). Azure Container
Apps read Log Analytics through an HTTP client that sent no credential at all
whenever the endpoint coordinate was `http://`, beside the SDK client used
otherwise, choosing between them at query time; `azurecommon.NewLogsQuerier`
now picks one client at construction from the coordinate, and the plain-HTTP
one requests the same Microsoft Entra token for the Log Analytics scope and
sends it itself (BUG-2949). The tests moved with the code — stdin pipe,
attach deadline, shared-volume parsing and translation, overlay image, pod
manifest, managed-volume shaping, network-pod materialisation, cloud-error
mapping, image reference splitting — and gained the cases the copies never
had; the per-backend suites keep what is per-backend (each `Config.Validate`
and the variable each backend reads). AGENTS.md carries the placement rule
("Shared code has three homes"), and the documentation links the Bleephub
extraction had left dangling now point at the Bleephub repository (BUG-2937).

Bundling the dependency drift the freshness gate reported moved the Go
toolchain forward: the latest `google.golang.org/api` and `golang.org/x/crypto`
require Go 1.26, so the modules that pull them declare `go 1.26.0`, and every
`actions/setup-go` pin, harness image, and smoke image builds with Go 1.26.
`setup-go` runs with `GOTOOLCHAIN=local`, so a runner left on 1.25 fails every
job at module load rather than downloading the toolchain.

## Azure build-context blob client authenticates with the account's shared key

The Azure Container Registry build service reached the build-context blob
container with `azblob.NewClientWithNoCredential`, on a comment claiming the
simulator "does not enforce storage bearer auth" — no longer true, so the
client would be refused at the next simulator pin bump, and it was never
right against a sovereign cloud either. The client now reads the storage
account's key through the Azure Resource Manager (`ListKeys`, the way an
administrator reads it) and signs with `NewSharedKeyCredential`, which works
over plain HTTP where azblob rejects bearer tokens — the same code against
the simulator and the real cloud, differing only in coordinates.

Proven end to end by a new test in backends/azure-common that provisions a
storage account through ARM against the pinned simulator, resolves the
advertised blob endpoint, and round-trips a blob through the signed client.
The blob host is a per-account name under .shim.localhost that a
deployment's DNS resolves (dnsmasq in the Linux harness, systemd-resolved on
CI); macOS cannot map it without root, the documented host-capability skip —
Linux never skips.

## Gave Azure Container Registry the credential it actually accepts

An Azure Container Registry does not accept a Microsoft Entra token on its
Docker Registry HTTP API v2 surface, and sockerless was presenting one. The
auth provider asked Microsoft Entra for `https://<registry>/.default` — an
audience Microsoft Entra does not issue, because Azure Container Registry has
no per-registry audience — and put the result straight on `/v2/` as a Bearer;
the multi-architecture index writer did the same with an Azure Resource
Manager token, beside a comment asserting that the token exchange was only
needed by tools wanting long-lived credentials.

The registry's documented flow now runs, all of it. A Microsoft Entra token is
acquired for the container registry *service* audience,
`https://containerregistry.azure.net/.default`; it is exchanged for an Azure
Container Registry refresh token at `POST /oauth2/exchange`; that refresh token
and the requested scope are traded for a scoped access token at
`POST /oauth2/token`; and that access token is the Bearer the data plane
honours. One token service behind the provider caches each hop until the `exp`
claim of the JWT the registry issued says it expires — the token service
reports no lifetime alongside its tokens, so the token itself is the only
statement of when it stops working — re-authenticates when a registry answers
401, and asks for exactly the access the operation needs in the Docker Registry
HTTP API v2 scope grammar: `pull` to read a manifest, `pull,push` to write one,
`pull,delete` to remove one, `metadata_read` to list a repository's tags, and
`registry:catalog:*` for the registry itself.

`core.AuthProvider.GetToken` carries the repository and the actions so that is
possible at all. The AWS and Google Cloud providers document the two arguments
as unused, because an Amazon Elastic Container Registry authorization token and
a Google Artifact Registry access token are registry-wide and carry whatever
the calling identity's policy allows.

Around that fix, the paths that reach a registry became one path. The registry
endpoint coordinate — the address an operator reaches a relocated registry at,
without changing the host its references name — was honored by tag and delete
and ignored by push, image listing, the multi-architecture index and the
Artifact Registry tag probe; `core.OCIRegistryBaseURL` resolves it for all of
them and `core.SetOCIHost` keeps the Host header naming the registry, which is
what a registry serving several login servers behind one address routes on and
what a token service matches its `service` parameter against. A failure to mint
a credential is no longer swallowed and retried anonymously in pull, push,
build or listing, because a registry that refuses the anonymous retry reports a
missing image rather than the credential problem behind it. Azure Container
Registry recognition covers the sovereign clouds' login servers rather than
only `.azurecr.io`.

The Azure Container Registry credential path is covered end to end against the
Microsoft Azure simulator, in `backends/azure-common`: a registry provisioned
through Azure Resource Manager, an image built as real gzipped layer bytes and
pushed over `/v2/`, a second tag added through the provider's own sync path,
the repository's tags read back with a `metadata_read`-scoped token, and both
tags removed — with an assertion that the credential presented is not the
Microsoft Entra token it was exchanged for, which is the defect stated as a
test. The simulator is reached only by coordinates: the registry endpoint, the
managed identity endpoint the Microsoft Entra token comes from, and the Azure
Resource Manager endpoint.

The Amazon Web Services simulator harnesses in `backends/ecs` and
`backends/lambda` reserve their own Amazon Route 53 resolver port instead of
leaving it on the default a host's own mDNS responder already owns, so they
start on a developer machine rather than panicking before serving anything.
Every harness image build passes `--load`, so its result reaches the container
runtime's image store rather than staying in a build cache the workload cannot
pull from — which had surfaced as five AWS Lambda tests timing out on a
callback from a container that was never able to start.

The simulators are pinned at the sockerless-cloud v0.9.2 release —
`tests/go.mod` at its release commit, the twenty-three harness image build
arguments at the same commit, and the git build context and live-test checkout
at the tag.

## Extracted the simulators into the sockerless-cloud repository

Moved the three cloud simulators — with their SDK/CLI/Terraform test suites,
console SPAs, vendored cloud API specifications, simulator gate
scripts/pre-commit hooks, CI jobs (unit/SDK/CLI/Terraform shards, fuzz
nightly groups, browser suites, quality gates), and the Firecracker/realexec
harness — to [sockerless-cloud](https://github.com/e6qu/sockerless-cloud) as
a fresh snapshot. There the per-cloud modules are installable
(`go install github.com/e6qu/sockerless-cloud/simulator-<cloud>@<tag>`), each
folding its former `shared/` module in as a package, with `realexec`,
`ui-auth`, and `testutil` as tagged support modules and the built console
`dist/` committed so installed binaries ship the console.

This repository consumes the simulators as pinned modules: `tests/go.mod`
pins the version via `tool` directives, the test harnesses, backend
integration tests, ECS Terraform module test, `test-shauth-rps.sh`, and the
stack targets build binaries from that pin into `tests/.build/`, harness
Docker images (smoke, gitlab, upstream act/gitlab-ci-local, e2e runners)
`go install` the modules at `ARG SOCKERLESS_CLOUD_VERSION`, the deploy
compose builds sim images from the pinned sockerless-cloud git context, and
`live-tests-lambda.yml` checks out the pinned tag for its live differential
suites. The `eval-arithmetic` and `container-command` workload fixtures moved
to `tests/testdata/` (the backends' integration tests and runner harnesses
use them). Simulator-side open bugs moved to sockerless-cloud's BUGS.md with
IDs intact; sim CI jobs, hooks, badges, goreleaser entries, GHCR sim image
publication, and the sim rows of the required-status-checks manifest were
removed here, and `main`'s branch protection was reconciled to the pruned
required-check manifest. A follow-up moved every pin to the v0.2.0
release-please release: `tests/go.mod` records release-commit
pseudo-versions (subdirectory modules cannot resolve a repository-root
tag), the harness Dockerfiles carry the release commit in
`ARG SOCKERLESS_CLOUD_VERSION`, the deploy compose and live-tests refs use
the `v0.2.0` tag, and the dependency-freshness gate skips the first-party
`github.com/e6qu/sockerless-cloud/*` modules whose proxy version list froze
at the deleted bootstrap tags.

## Compressed History

- **August 2026 — extraction and registry authentication.** The three cloud simulators, their SDK/CLI/Terraform suites, consoles, vendored specs, gate scripts, and the Firecracker/realexec harness moved to [sockerless-cloud](https://github.com/e6qu/sockerless-cloud); this repository pins them as Go modules and builds them into `tests/.build/`. Azure Container Registry received the credential it accepts (the registry's own token service, BUG-2938), every registry operation honoured the registry endpoint coordinate (BUG-2939), credential failures stopped being retried anonymously (BUG-2940), and the build-context blob client signed with the storage account's shared key. Before the extraction, the same branches closed the simulator-side waves whose detail now lives in sockerless-cloud: resource-scoped IAM authorization derived from published ARN formats, response-pattern conformance on all three clouds, Amazon ECS service scheduling and task-role credentials, Azure virtual machines and Network Watcher over real guests and real capture, Google Compute Engine packet mirroring, and the persistence audit that made cloud state survive a simulator restart.
- **July 2026 — consoles, identity, and the runner topologies.** The AWS, Google Cloud, and Azure simulator consoles adopted their real component libraries and reached functional parity through the clouds' own APIs; Shauth OpenID Connect sign-on sat on top of cloud-faithful authentication with federation through each cloud's own primitive; `sockerless login` signed the terminal into every cloud; consoles minted real CLI credentials and managed accounts, projects, and subscriptions. The simulators started verifying caller credentials on their data planes (BUG-2625). Bleephub and Bleeplab were extracted to their own repositories on 15 July. Both CI runner topologies (GitHub Actions runner, GitLab docker-executor) were proven on every container-capable backend against the simulators.
- **Earlier — the backends and the foundation.** Stateless cloud backends for Amazon ECS, AWS Lambda, Google Cloud Run, Google Cloud Run Functions, Azure Container Apps, and Azure Functions, each assembling networks (Cloud Map, Cloud DNS, Private DNS), multi-container pods (multi-container revisions, Container Apps, Fargate tasks, function pods with a supervising bootstrap), and volumes (EFS access points, Cloud Storage buckets, Azure Files shares) from cloud primitives plus the sockerless agent in forward and reverse mode; the typed `api.Backend` contract with its compiler-enforced explicit implementations; the storage-backing driver registry; the Docker passthrough backend; the CLI, admin console, and runner dispatchers.

## Foundation Summary

- Docker-compatible cloud backends are stateless and map Docker concepts onto cloud primitives; shared behaviour has one implementation in `backends/core`, `agent/envelope`, or the owning `*-common` module.
- The AWS, Google Cloud, and Azure simulators are real cloud API slices, developed and gated in sockerless-cloud and consumed here as pinned modules.
- GitHub Actions runner and GitLab docker-executor topologies are simulator-proven across container-capable backends; live-cloud validation beyond AWS Lambda remains open under BUG-1075.
