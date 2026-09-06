# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact history.

## The registry checks credentials now, and so does every path that reaches it

sockerless-cloud v0.30.3 made Google Artifact Registry's data plane refuse
what the real one refuses: an anonymous `/v2/` request, a credential it did
not issue, and a push into a repository nobody created. Building against that
release (pinning `ui-auth` at the release commit too — its deleted `v0.1.0`
bootstrap tag outranks every pseudo-version, so a stale pin had been
building the new simulators against the old package) turned three filed
defects into failures, and this branch closed them. The pin itself stays at
v0.9.2: the CI run against v0.30.3 proved the provisioning — repositories
created, the login accepted, the harness's pushes and the Cloud Build push of
the overlay all succeeded — and then showed that release's own Cloud Run and
Cloud Functions hosts pulling the workload image with no credential from the
registry they now enforce (BUG-2951), which no harness can supply for them.

The Artifact Registry coordinate had been two settings that agreed by
accident: the auth provider took the Google Cloud API endpoint, while the
overlay host, the tag probe, and the resolver each read
`SOCKERLESS_GCP_AR_ENDPOINT` from the environment on their own, and the probe
defaulted a bare `127.0.0.1:<port>` to `https://`, failed, and reported the
failure as a cache miss — a silent Cloud Build on every overlay start. Both
Google backends now read the coordinate once into `Config.ARRegistryEndpoint`
and pass it everywhere: `gcpcommon.OverlayRegistryHost` names the host
references carry, `gcpcommon.RegistryEndpointURL` the URL registry HTTP is
dialed at, `IsRelocatedRegistry` lets the auth provider recognise the
relocated host as its own registry, and `CheckTagExists` answers
`(bool, error)` with only 200 and 404 as answers. A Cloud Build service that
cannot be constructed fails the backend at startup instead of vanishing.

The shared `BaseServer.ImagePull` had fetched metadata anonymously with an
`auth` parameter it ignored, `ImageManager.Pull` passed the client's
credential nowhere for a registry that is not the backend's own, and
`ImagePushToEndpoint` passed it verbatim as if it were a token. One decoder,
`core.RegistryAuthorizationFromDockerAuth`, turns the Engine API `AuthConfig`
in `X-Registry-Auth` into the `Authorization` value the registry accepts —
username and password to a Basic credential presented to the token service
the way the Docker CLI presents it, `registrytoken` to the registry's Bearer,
`identitytoken` refused rather than downgraded, a minted cloud token passed
through — and a registry that challenges with `Basic` gets the credential
directly. A core test runs a TLS registry with a token service and a
Basic-only repository and pulls a private image with the client's credential
after the anonymous pull fails.

The Cloud Run and Cloud Run Functions harnesses had pushed with no login into
repositories that did not exist. They now provision the way Terraform and the
operator do against the real service, through `gcp-common/registrytest`: the
service-account key first, an access token minted from it by the JWT-bearer
grant at the key's own `token_uri`, the `docker-hub` and `sockerless-overlay`
repositories through the Artifact Registry API, and `docker login -u
oauth2accesstoken --password-stdin` — in a Docker configuration directory of
the harness's own that the simulator process inherits, because its Cloud
Build executor and its Cloud Run host push and pull with the simulator's
Docker CLI the way a Cloud Build worker and the Cloud Run service agent carry
the project's registry credential. The backend's coordinate carries its
scheme, and the smoke, GitLab, e2e, and `make stack-*` simulator stacks export
it — the one registry coordinate now decides where registry HTTP is dialed,
where before the API endpoint had quietly doubled as it. The Terraform
modules for both Google backends gained the `sockerless-overlay` repository
the overlay path pushes to and the `gitlab-registry` remote repository the
resolver names; neither existed for a live deployment before.

The same run surfaced two smaller defects. The Azure backends had listed
repository tags for `docker images` with `metadata_read`, an action of Azure
Container Registry's own `/acr/v1/` surface; a `GET /v2/<repo>/tags/list` is a
read of the repository the registry challenges for as `pull`, and both
backends and the round trip now ask for that. And the `make tf-int-test-*` targets
could not run at all — the image was built without `--load`, the entrypoint
was handed a flag it does not take, and no Docker socket was mounted. With
the registry coordinate alone deciding where registry HTTP is dialed, the CI
smoke container also had to change shape: its simulator serves the registry
at its own loopback address, and the host daemon that runs the workloads
must share that loopback to pull from it, so the smoke and Terraform harness
containers run with `--network host`, the topology the Linux integration
harness already has. The Terraform harness then reached Terragrunt apply and
failed there, and the repair kept going: the provider's token minted from the
simulator's own token endpoint and handed over as the provider's
`GOOGLE_OAUTH_ACCESS_TOKEN`, the project created through the Cloud Resource
Manager API, every client of the provider routed at the simulator (the IAM
beta client `google_service_account` speaks through had reached the real
service), every coordinate the backends require exported from the outputs and
the host, the Lambda environment standing alone instead of reading the live
ECS environment's state from a real bucket, and an image that carries `act`,
the Docker and AWS CLIs and the bootstrap binaries with a workflow to run.
Amazon ECS runs end to end on this host; a `terraform-integration` CI job now
applies every environment, so the harness cannot decay unseen again
(BUG-2955). Its Google cells found a bootstrap defect no other harness had:
act starts its job container in a working directory the image does not
hold, Docker creates such a directory at start, and the bootstraps did not,
so the workload died at `chdir` and every exec after it was refused. The
bootstraps now create it (BUG-2958). The same cells then found that the
reverse-agent `docker cp` into a container required the destination to
exist, where Docker and the ECS SSM path create it; act copies into
`/var/run/act/` unprepared, and the put-archive exec now runs `mkdir -p`
first (BUG-2959). With that in place act ran its step, and the harness
showed the ECS cell had been green on a falsehood: `docker exec` on Amazon
ECS reported exit 0 whatever the command returned, because the ExecuteCommand
stream decoder ended the session without recording a status. A non-TTY exec
now prints the exit marker the one-shot SSM path already used and the decoder
strips it before the client sees it; a TTY exec takes the session's
exit-code frame (BUG-2960). The same run showed act starts every step with
`bash` on a platform image, so the smoke workflow names `sh`, the shell
alpine has, and that the pinned simulator drops an ECS container definition's
`workingDirectory` — fixed in sockerless-cloud, so the ECS cell passes again
once the pin carries it. The Cloud Run cell then showed the other side of
the same directory: a pod Service is only warmed for `docker exec` until
stdin arrives, so nothing had created the container's working directory
when act exec'd into it; every bootstrap now creates it at startup, as
Docker does at container creation (BUG-2961). The ECS cell had stayed green
through the simulator's missing directory because its exec script let a
failed `cd` fall through to the command; the script now fails the exec
with status 126 as Docker does, so that cell is red until the pin carries
the simulator fix (BUG-2962). sockerless-cloud shipped that fix in v0.30.7
and the pin moved there from v0.9.2 — the release also carries the Google
hosts pulling as the service agent, the owner-aware subnet reclaim and the
Azure v2 catalog — so every cell is green. And an e2e run lost a port race the harnesses
had always been able to lose — each port probed and released before the
next, so the simulator's DNS listener was handed the port just chosen for
the backend; `core.PortReservation` now holds every port of a harness until
the whole set is chosen.

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
