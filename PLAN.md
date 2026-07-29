# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators. Bleephub is an independent GitHub Enterprise Server-compatible service and consumes Sockerless through its published simulator/backend integration contract.

## Non-Negotiable Principles

1. Match public application programming interfaces exactly: Docker, GitHub, and public cloud APIs.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, degraded modes, or skip-if-tool-absent tests.
3. Cloud backends stay stateless; the cloud is the source of truth.
4. Simulators are real local cloud slices, one binary per cloud, validated through official SDK, command-line interface, and Terraform clients where applicable.
5. Bleephub metadata/index state belongs in SQLite; git objects, artifacts, caches, release assets, packages, job logs, and similar durable bytes belong in object storage or an explicit durable local-development filesystem.
6. Public/user-facing Bleephub behavior must use GitHub-shaped public paths and contracts, not `/internal/*` operator shortcuts.
7. The user merges PRs. Agents create branches, commits, and PRs only.

## Active Focus

**Cloud simulator/backend fidelity, production operation, and live-cloud validation.**

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator dashboards completed one first-party OpenID Connect boundary for their rendered interfaces and operator data while native cloud protocol routes remained independent. The exact PostgreSQL, Ory Hydra, Shauth, compiled relying-party, and Chromium matrix proved direct and catalog entry, shared sign-on, identity, relying-party and provider logout, correlated logout completion, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and validator-credential isolation.

Production operation had enforceable resource and artifact contracts. Every ordinary GitHub Actions job was bounded to 15 minutes, the historically over-budget AWS edge and Amazon EC2 command-line interface groups were split without losing any of the 630 tests, nightly fuzz targets ran in bounded parallel batches, and clean production builds created every frontend before compiling all 11 UI-bearing Go binaries. Native release tags remained direct architecture manifests while each short-SHA tag remained an OCI index containing exactly Linux ARM64 and AMD64.

The fidelity work stayed evidence-driven. AWS Lambda and AWS Step Functions covered every operation in their vendored Smithy service models with executable implementations. The follow-on sweeps closed Amazon SQS runtime semantics, Amazon EC2 subnet dependencies and sparse snapshots, Amazon ECS `StartTask` and launch-type sandboxing, real Amazon Amplify builds, the AWS Certificate Manager ACME service, AWS Private Certificate Authority, Amazon Data Firehose, SMTP-backed Amazon SNS email delivery, Firehose-backed Amazon SNS and Amazon CloudWatch delivery, and repeated AWS/Microsoft Entra OpenID discovery. Google Cloud closed the described-but-unserved cryptographic, rotation, Autokey, Memorystore, and Cloud Run projection gaps; Azure Files gained Share ACL. Official SDK, vendor CLI, Terraform, RFC 8555, SMTP, Git, container, authenticated browser, and external reverse-proxy clients proved the public contracts externally.

## Active Branch Priorities

1. Implemented the complete 23-operation AWS Private Certificate Authority slice and connected it to AWS Certificate Manager private issuance, export, and revocation.
2. Implemented the complete 12-operation Amazon Data Firehose slice with durable encrypted buffering, real IAM/KMS enforcement, Amazon S3 delivery, and Amazon SNS and Amazon CloudWatch producers.
3. Added production AWS console workflows for Firehose delivery streams and root private certificate authorities, validated through the authenticated Shauth browser matrix.
4. Removed repeated OpenID discovery from AWS Security Token Service and Microsoft Entra federation while preserving complete per-assertion verification.
5. Made the deployment reverse proxy report bounded cold-start `503 Retry-After` responses and preserved sparse Amazon Elastic Block Store extents through snapshot, restore, and copy.
6. Regenerated public surface tables and assigned every new official AWS CLI test to exactly one continuous-integration shard.
7. Upgraded every same-day AWS SDK dependency drift in the open branch and retested all affected modules and service wire paths.
8. Added optimized and SDK AWS Step Functions integrations for Amazon ECS and AWS CodeBuild, including synchronous, callback, failure, timeout, stop, and real-container cancellation lifecycles.
9. Made AWS CodeBuild authenticate private Git revisions, execute checked-in or explicit build specifications inside the exact configured image, and cancel build and build-batch containers on public stop operations.
10. Made AWS Amplify retain encrypted connected-repository credentials and execute complete authenticated multi-language monorepo phase, environment, cache, and artifact lifecycles.
11. Added persistent PostgreSQL, MySQL, and MariaDB Amazon Relational Database Service data planes with native TLS, encrypted master secrets, TLS-only IAM database authentication, and live or pending password rotation.
12. Upgraded every same-day Google Cloud API dependency drift, fixed Buildx external-test image loading, and reran the complete affected SDK, CLI, Terraform, browser, and production-build gates.
13. Migrated Azure Container Apps, Azure Functions, and the production-shaped Azure simulator Terraform stack to HashiCorp AzureRM 5.0.0, including every provider-required resource-ID field.
14. Made region-skewed Google Discovery drift retain the exact upstream documents as short-lived CI artifacts, and replaced CloudWatch metric-stream test placeholders with real Amazon S3, IAM, and Amazon Data Firehose resources.
15. Kept the Azure Terraform integration budget available for the official provider by installing Ubuntu's signed Caddy package through the existing bounded APT path.
16. Round-tripped Microsoft.Network subnet `addressPrefixes` from AzureRM 5 into the real network fabric and kept failed Azure portal deletions attached to their accessible confirmation dialog.
17. Made the production Azure Container Apps module and external provider stack select AzureRM 5's required `log-analytics` destination when linking a Log Analytics workspace.
18. Corrected the AWS Lambda module's Step Functions live-differential IAM policy to use its declared region input and revalidated every production Terraform module.
19. Restored canonical HCL formatting across the complete Terraform tree.
20. Advanced five Google Discovery documents from the exact hosted-runner artifacts, implemented the newly published Bigtable memory-layer update and Cloud Resource Manager resource-semantics methods, and ratcheted their measured coverage floors.
21. Made Microsoft.OperationalInsights network-access defaults and Microsoft.Storage File-share access policies survive AzureRM 5 apply and refresh through the real ARM contracts.
22. Updated the external AzureRM 5 post-plan assertions to its canonical Microsoft.Storage ARM resource IDs.
23. Added Cloudscape operating workflows for AWS CodeBuild projects and builds, AWS Amplify branches and deployments, and Amazon RDS authentication, then proved them through the authenticated browser matrix.
24. Documented and externally proved standard AWS endpoint propagation from explicitly deployed AWS Lambda and AWS CodeBuild workloads.
25. Retained the exact CI-captured Cloud Logging v2 and IAM Service Account Credentials v1 Discovery revisions and revalidated the unchanged public method, path, and schema-field sets.
26. Closed the hosted-runner concurrency findings: AWS Amplify preserved sub-second release ordering, Microsoft Azure NAT gateways accepted subnet association before public addressing, and the real AWS Step Functions workload test allowed cloud-shaped cold container provisioning.
27. Upgraded the newly released SQLite and Google Cloud client graphs, including canonical Firestore and Spanner protobuf modules, and reran every affected module plus the complete official Google Cloud SDK suite.
28. Retained the exact CI-captured Cloud Run v1 and v2 Discovery revision 20260727 documents and revalidated their unchanged public method, path, and schema-field sets.
29. Provisioned the exact public Amazon ECS and AWS CodeBuild workload images before the AWS SDK shard's per-test lifecycle deadline, then reran the real-container integration.
30. Made all three console skip-link tests start keyboard traversal from deterministic in-document focus and reran them in real Chromium.
31. Preserved explicit Amazon ECR Public workload coordinates, made cancellation terminate CodeBuild containers on every Docker wait path, and externally proved the stopped build emitted no delayed Amazon SQS message.
32. Loaded Docker Buildx test images, shared the container host PID namespace for real VPC attachment, and completed the full production-shaped AWS Terraform graph through HTTPS.
33. Loaded the Amazon ECS arithmetic workload through the backend's Docker Image Load API, required an explicit Amazon ECR workload coordinate for live-cloud runs, and passed all six real-container lifecycle cases.
34. Preserved the AWS Signature Version 4 host through the HTTPS gateway, serialized heavyweight Terraform packages locally, isolated all five production-shaped graphs on separate hosted runners, and passed every HTTPS apply, real workload or data-plane assertion, and destroy.
35. Upgraded the newly published `go-git` patch and its current transitive graph, passed the complete AWS simulator module suite, and reran the authenticated freshness audit.
36. Loaded the shared compiled arithmetic fixture through every active cloud backend's Docker Image Load API and passed both the exact e2e suite and the optional second Amazon ECS simulator-backend path.
37. Upgraded both immutable multi-architecture publication jobs to the newly released `docker/login-action` 4.6.0 and passed action syntax, publication-contract, and authenticated freshness validation.
38. Kept native Linux workload endpoints on Docker's host-gateway alias, reserved gateway rewriting for a containerized simulator, and passed the real Step Functions → Amazon ECS → AWS CodeBuild → AWS CLI integration.
39. Declared HashiCorp AWS provider 6.50.0 across every simulator Terraform graph, passed the complete root graph through HTTPS with runtime Smithy validation, and made the Microsoft Azure failed-delete UI assertion await its retained accessible dialog.
40. Made Google Cloud Spanner transactional over official REST and gRPC clients, with strict DDL, composite-key behavior, real SQLite transactions, CLI SQL, and provider apply/zero-plan/destroy coverage.
41. Ran the official HashiCorp Terraform image inside a Step Functions-launched synchronous Amazon ECS task and applied Amazon SQS through the standard global AWS endpoint.
42. Completed AWS Amplify build/test artifact and retry projections, connected WAF association to hosted traffic and sampled requests, and made repeated AWS Certificate Manager DNS validation stable.
43. Converged API Gateway v2 and AWS Lambda state under an unmodified ecs-dev-desktop Terraform graph that applied 178 resources, produced a zero-change plan, and destroyed all 178 with no Smithy violations.
44. Upgraded Google Cloud Spanner to v1.94.0 and the complete affected AWS SDK graph to v1.43.2/current service releases, then passed both official SDK suites and the authenticated freshness audit.
45. Removed the Google Terraform capability skip, loaded the Buildx test image into the runtime, and installed Firecracker plus squashfs tooling in the shared Linux test image.
46. Recorded the remaining real boundaries as BUG-2764 through BUG-2767 instead of hiding them behind skips, HTTP 501 responses, unbound listeners, or partial WAF semantics.
47. Persisted AWS Key Management Service custom policies across SQLite reads and simulator restarts, with focused durable-store and production-shaped HashiCorp AWS provider coverage.
48. Refreshed the coordinated AWS SDK patch wave, Google Cloud Spanner client, and resolved transitive graphs across every affected Go module after the pre-push freshness gate detected their publication.
49. Made filesystem staging validation privilege-independent by forcing the direct destination beneath a regular file instead of assuming `/usr/local` was unwritable.
50. Hardened the Terraform-in-ECS provider boundary with exact-image pre-pull, bounded provider-initialization retry, Amazon CloudWatch task output, immediate terminal-failure reporting, and repeated exact-shard validation.
51. Kept continuity concise and current; detailed historical work remained in pull requests and `git log`.

## Verified Next Gaps

1. BUG-2712 retained only mobile-push-provider and carrier-backed SMS delivery; SMTP and Amazon Data Firehose transports were complete.
2. BUG-1075 retained authenticated validation for the remaining real-cloud backend cells.
3. BUG-2646 retained only Google's upstream Cloud Run Discovery publication lag.
4. BUG-1345 retained only the upstream AzureAD provider's lack of a Microsoft Graph endpoint override.
5. BUG-2523 and BUG-2441 remained in the external Bleephub repository rather than this workspace.
6. BUG-2767 retained complete AWS WAF statement evaluation; BUG-2766 retained AWS Amplify Hosting image optimization; BUG-2765 retained occupied-port load-balancer listeners on macOS; BUG-2764 retained the macOS Podman nested-KVM boundary.

## Simulator Console Parity

The three simulator interfaces adopted distinct cloud-native shells and
component systems: AWS used Cloudscape, Google Cloud used its Material console
idiom, and Microsoft Azure used Fluent UI portal patterns. Cloud-neutral API
and authentication behavior remained shared while each cloud owned its
navigation, typography, density, actions, resource tables, detail treatment,
empty states, and themes.

The goal is recognisability, not replication: adopt each cloud's real shell,
information architecture, density, colour, and terminology, using each
vendor's published design guidance, without copying proprietary assets.

Evidence gathered by opening both sides and capturing them, rather than from
memory:

- **AWS** builds its console from the Cloudscape Design System, which is
  published. Its resource pages carry a dark global header, breadcrumbs, a
  collapsible grouped service navigation, a `Resource (count)` heading with an
  information link, a primary create action beside secondary row actions,
  per-column sorting and filtering, selection checkboxes, per-row overflow
  menus, pagination, density settings, and dismissible notifications. Its
  dashboards carry region context, service health, and charts.
- **Azure** publishes an annotated diagram of its portal shell: a dark global
  header, breadcrumb, global search, a horizontal command bar of icon actions,
  a service menu with its own search and expandable groups, and an Essentials
  panel presenting resource properties as a two-column key/value grid.
- **Google Cloud** presents a light global header — hamburger, wordmark, a
  project picker chip, and a wide central search field, with the account and
  tools at the right — over a product navigation whose active item is a filled
  rounded pill. Its resource pages put inline text actions beside the page
  title rather than a button group, describe the resource in a sentence beneath
  it, and carry a filter chip with column settings above a table whose headers
  hold inline help. Empty states pair a hand-drawn illustration with a headline,
  an explanation, the side effect of the action, a filled primary button, and a
  quickstart link. The console itself offers Light, Dark, and Same-as-device
  themes plus an increased-contrast setting.

The three cloud-specific passes were reviewed against their corresponding
console rather than against one another.

1. **AWS simulator console parity.** Shell, navigation, resource tables, and
   empty states in the Cloudscape idiom. Done.
2. **Azure simulator console parity.** Portal shell, command bar, service menu,
   and Essentials-style resource properties. Done.
3. **Google Cloud simulator console parity.** Light header with its project
   chip and central search, inline text actions beside the page title, the
   filter chip with column settings, and the illustrated empty state. Done —
   all three simulator interfaces now present as their own cloud's console.

Every interface carries a light and a dark theme, a theme control in the top
right of the header where each console keeps its own, and text that meets WCAG
AA contrast in both. Contrast is measured against the rendered surfaces rather
than assumed from the palette.

Shared shell components currently live in `ui/packages/core`. Divergence is the
point here, so per-cloud presentation belongs in each simulator package, and
only genuinely cloud-neutral behaviour stays shared.

## Simulator Console Real-API Federation

The console interfaces now look like their clouds but still read data through
sockerless-invented `/sim/v1/<service>` endpoints that return a trimmed,
hand-picked shape (BUG-2635). The faithful end state is what a real cloud
console does: the browser calls the real cloud API paths with real short-lived
credentials, obtained by federating the Shauth-authenticated operator session.
Shauth OIDC SSO sits on top as the identity the federation consumes — additive
to what the cloud UI natively needs, exactly as an enterprise wires an external
identity provider into a cloud console.

The mechanism per cloud is the cloud's own federation primitive: Google Cloud
Workforce Identity Federation (an STS token exchange that takes an external
OIDC assertion and returns a federated access token), AWS
`AssumeRoleWithWebIdentity` (temporary credentials the browser signs requests
with), and Microsoft Entra federation into an Azure Resource Manager token. The
console backend already holds the operator's Shauth ID token in the ui-auth
session, so it brokers the exchange and hands the browser only a short-lived
cloud token; the raw assertion never leaves the server. The credential-issuance
and validation this needs overlaps and advances BUG-2625.

One slice per cloud, one pull request each:

1. **Google Cloud — real Cloud Run API.** The sim's STS token-exchange endpoint
   verifies the operator's Shauth assertion against a workforce pool provider
   and issues a federated access token; the Cloud Run `v2` job endpoints
   validate that token; the console obtains it from the session broker and reads
   the real `GET /v2/.../jobs` (list) and `GetJob` (detail), rendering the true
   `Job` resource; the `/sim/v1/cloudrun` route is deleted. The workforce pool
   and provider CRUD the exchange configures already exist in the GCP sim. Done
   — the Security Token Service token exchange, the session broker, the console
   credential path, and the real Cloud Run jobs list and detail all landed. The
   remaining Google Cloud resources (functions, Artifact Registry, Cloud
   Storage, Logging) follow the same pattern next.
2. **Amazon Web Services — real ECS/Lambda API** via
   `AssumeRoleWithWebIdentity` and browser-side Signature Version 4. Done.
3. **Microsoft Azure — real ARM API** via Entra federation into an ARM bearer
   token. Done — no `/sim/v1/*` dashboard route remains on any console.

Each slice deletes its cloud's `/sim/v1/*` routes only once the real path is
proven, keeps the resource detail pages the real Get APIs make possible, and
carries the SDK, CLI, and Terraform coverage the simulator testing contract
requires for every new endpoint.

## Console Self-Service: Credentials, Accounts, Login, Deployment

Shauth SSO sits on top of — never replaces — cloud-faithful authentication: a
user signs into Shauth once, the console federates that identity through each
cloud's own primitive, and everything past federation is the real cloud wire
contract. On that foundation, the consoles become self-service: a
Shauth-authenticated user mints the cloud credentials their vendor CLI needs
and manages the accounts and projects those credentials live in, exactly as
the real consoles allow. Four phases, one branch and pull request each, in
this order.

1. **Credential-minting console pages.** Each console gains its cloud's real
   credential page, driven by the operator's federated credentials calling the
   cloud's real APIs — never a console-only endpoint: AWS Identity and Access
   Management users with the "Create access key" flow (the Security
   credentials page); Google Cloud service accounts with key creation and the
   one-time `privateKeyData` JSON download; Microsoft Entra ID app
   registrations with client-secret creation (the Certificates & secrets
   blade). Each page shows the exact vendor-CLI usage for the minted material
   (`aws configure`, `gcloud auth activate-service-account`,
   `az login --service-principal`). Done — and the phase turned out to be more
   than UI: proving the loops end to end surfaced that the Google OAuth 2.0
   token endpoint never verified assertion signatures (minted keys' public
   halves are now registered, verified, and revocable) and that the Microsoft
   Entra v2.0 token endpoint validated no client secret (directory-registered
   applications now carry hashed password credentials that
   `client_credentials` verifies with real AADSTS failures); the invented
   `/sim/v1/entra/users` seed routes were deleted for Graph provisioning. CLI
   tests prove each minted credential authenticates the vendor CLI, and the
   Shauth relying-party browser matrix mints on the AWS and Google Cloud
   consoles with one-time disclosure asserted. The Azure portal's browser
   minting flow is staged into phase 4: its browser-side Workload Identity
   Federation exchange is same-origin-only (real Microsoft Entra serves no
   CORS for `client_credentials`), so the separately-deployed console needs
   the server-side federation broker, faithful Azure Resource Manager and
   Microsoft Graph CORS, and a pre-provisioned portal identity (BUG-2640).
2. **Account and project management.** Two new simulator slices with the
   mandatory SDK, CLI, and Terraform tests: Google Cloud Resource Manager
   (`projects.create`/`list`/`delete`) and the Azure `Microsoft.Subscription`
   alias API (subscription creation). Console pages on top: AWS Organizations
   account list/create (the Organizations slice already exists), the Google
   Cloud project picker with New Project and project management as the real
   console header offers, and Azure subscription list/create. Privilege comes
   from real cloud authorization on the federated principal — Shauth roles map
   to differently-privileged cloud principals through the federation resources
   (role trust policy, workforce pool, federated identity credential), never a
   bespoke sockerless permission check. Done — and the Google Cloud slice
   replaced a faked partial v3 surface (projects synthesized on sight,
   synthetic operations) with the real contract; the Azure subscription
   Terraform coverage runs as its own `tf (azure subscription)` shard because
   the provider's fixed settle delays don't fit the shared azure stack's
   budget.
3. **`sockerless login`.** The packaged terminal analog of
   `aws configure sso` / `gcloud auth login` / `az login`: browser sign-in to
   Shauth via a localhost callback, the per-cloud federation exchange, and the
   resulting real cloud credentials written to the vendor tools' standard
   locations (`~/.aws/credentials`, Application Default Credentials, the az
   token cache) so unmodified vendor CLIs and SDKs work against the deployed
   simulators. Done — implemented as vendor-native credential wiring the
   tools refresh themselves (AWS `web_identity_token_file` profile, a
   workforce `external_account` ADC file, `az login --federated-token`), with
   the CLI as a public Hydra client over the RFC 8252 loopback flow; proving
   gcloud's path added the simulator's missing STS token-introspection slice
   (BUG-2641).
4. **Deployment and provisioning recipe.** Committed infrastructure that hosts
   Sockerless Admin, the three simulators, and a Shauth instance at persistent
   origins: each console registered as a Shauth OpenID Connect client with its
   real redirect URI, the federation resources provisioned via Terraform or
   the real APIs, and a live-origin smoke test that signs in via Shauth and
   reads each console's data plane. The multi-architecture images already
   published are the artifacts; the coordinates and orchestration are the
   deliverable. This phase also carries BUG-2640: the Azure portal's
   federation exchange moves into the console's server-side broker (real
   Microsoft Entra serves no CORS for `client_credentials`, so the browser-side
   exchange only works co-served), the simulator gains faithful Azure Resource
   Manager and Microsoft Graph CORS, and the relying-party harness runs the
   Azure console and cloud as separate processes with the portal's managed
   identity provisioned before console start — unlocking the Azure browser
   minting flow deferred from phase 1. Done — `deploy/` boots the Shauth
   stack + Admin + the three simulators behind a Caddy TLS proxy with
   real-API provisioning and a smoke gate, and the Azure portal federates
   through the console's server-side broker with faithful ARM/Graph CORS, the
   harness running the Azure console and cloud as separate processes.

## Standing Work

- **Bleephub full-service parity:** continue closing REST, GraphQL, UI, runner, auth, Pages, release, packages, and repository-provider gaps until Bleephub is usable as a real GitHub-compatible service.
- **BUG-1075 live-cloud validation:** the local-cloud runner cells are sim-proven, but live authenticated cloud validation remains open.
- **BUG-1345 AzureAD Terraform provider upstream blocker:** add AzureAD Terraform tests only after the provider supports a Microsoft Graph endpoint override.
- **BUG-2441 current `knip`/Node deprecation warning:** the unused-export gate passed, but the current `knip` 6.23.0 release still emitted Node's `DEP0205 module.register()` warning.
- **Issue #363 versioned releases + GitHub Container Registry images:** still a release/distribution task.
- **Simulator service ratchets:** AWS/GCP/Azure have operation-coverage gates that measure served surface (BUG-2651); continue ratcheting uncovered cloud services when Bleephub focus is not the immediate task. Google Cloud Spanner's REST session data plane was completed in this branch; Cloud Billing and the Azure Blob/File/Queue operations that declare a `501` gap remained among the largest measured surfaces.

## Compressed Foundation Summary

- The cloud backend family is Docker-API-shaped and stateless across Docker passthrough, Amazon Elastic Container Service, AWS Lambda-class, Google Cloud Run, Google Cloud Run Functions, Azure Container Apps, and Azure Functions.
- GitHub Actions runner and GitLab docker-executor topologies are sim-proven across the container-capable backends, including container jobs, service containers, artifacts, and dispatcher-spawned runners.
- FaaS multi-container pod semantics were assembled from cloud primitives, including shared-loopback networking and shared workspace behavior.
- AWS, GCP, and Azure simulators have conformance/coverage gates that measure served surface: a documented operation counts only when a probe of the running simulator reaches a handler that answers, or — on Amazon Web Services — when the registrar that mounted it is the service being credited. Earlier per-service "100%" figures predate that measurement and were inflated by pattern collisions; the current floors are the honest ones. Historical per-service detail lives in the merged PRs.
