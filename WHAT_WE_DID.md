# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact foundation summary.

## 2026-07-26 — The consoles now tell the truth about what the simulators support

The three simulator consoles were badging most of their cloud's catalogue "Not
supported" while the simulators demonstrably implemented it. The AWS simulator
alone registers roughly two thousand operations across forty services, yet the
console marked EC2, DynamoDB, RDS, KMS, Route 53, API Gateway, SNS, SQS,
Systems Manager and more as unavailable. This pass derived the true map from the
simulators themselves — reading route registrations rather than file names, and
in two cases dumping the router at runtime and probing a live simulator with a
real token — then corrected every catalogue and built the missing pages.

- **AWS** — 15 of 22 "Not supported" labels were false. Fifteen further
  implemented services were absent from the catalogue entirely, and nine
  genuinely-unimplemented ones were added so the catalogue reads complete rather
  than silently omitting them. Thirty list pages and six detail pages now cover
  33 services on their real operations. A mechanical check confirmed all 89
  operations the console calls were already registered, so no simulator change
  was needed — the honest outcome rather than an invented one.
- **Google Cloud** — 19 products were falsely badged; four implemented products
  were missing from the catalogue. Twenty-one list and fifteen detail pages were
  added, with writes driven through real long-running-operation polls. One
  genuine simulator gap was filled: the regional `compute.subnetworks.list` 404'd
  while its aggregated sibling worked. `supported` is no longer asserted — it is
  derived from whether a product has a page, so nothing can claim support it
  does not have.
- **Microsoft Azure** — unsupported went from eleven to seven, with 29 new
  blades covering virtual machines, App Service, Cosmos DB, PostgreSQL, Redis,
  virtual networks, load balancers, network security groups, DNS, Key Vault,
  Service Bus, Event Hubs, Event Grid, API Management, Logic Apps and more. Two
  simulator gaps were filled (`VirtualMachines_ListAll` and
  `VirtualMachines_Update`) with SDK and CLI coverage. A follow-up corrected a
  naming error the audit exposed — the menu entry labelled "Container Apps"
  actually pointed at Container Apps *jobs* — so both now appear as the distinct
  services they are.

Screenshots of every console in both themes caught two rendering defects that
the structural, axe and contrast suites all missed. The Google Cloud header
search field carried a hard-coded light fill while its text followed the theme
token, leaving near-white text at 1.08:1 in dark mode — invisible while typing;
it now uses a themed token and measures 10.79:1. Azure's "Not supported" badge
had no style rule at all, so in the narrow service menu it wrapped onto two
lines and collided with the service name. A third defect found the same way: an
unknown `/ui/...` path rendered an empty shell in all three consoles, which now
redirect to the overview.


## 2026-07-26 — Closed the fidelity gaps the test-contract pass surfaced

The test-contract pass filed four follow-ups rather than dropping them; this
closes three and narrows the fourth to its genuine upstream cause.

- **AWS host-prefix accommodations (BUG-2648).** Three sdk-test clients
  suppressed the endpoint host prefix their operations model — Cloud Map's
  `data-`, Step Functions' `sync-`, and CloudWatch Logs' `stream-` (the third
  was not in the original report) — so the suite proved a simulator-special path
  rather than the real client path. All three now use stock clients at
  service-shaped endpoints plus a shared transport overriding only
  `DialContext`: the SDK builds and signs a byte-identical request, and only the
  dial destination differs. Two macOS `t.Skip`s in the CLI suite were removed
  the same way through an `HTTP_PROXY` coordinate, so four operations that had
  never run outside Linux CI now execute everywhere. Guard tests capture the
  signed request after the finalize step, so re-introducing the accommodation
  fails the suite.
- **Cloud Run container fidelity (BUG-2647).** `Container` now models the three
  probes and `EnvVar` models `valueSource`, with the probe/HTTP/TCP/gRPC action
  and secret-selector types taken field-for-field from the vendored Discovery
  document. Because these are the shared v2 types, Jobs, Services, Worker Pools,
  and Instances all inherit them. `EnvVar` also marshals its `values` oneof
  correctly — a sourced variable returns `valueSource` and no `value`, where the
  simulator previously always emitted an empty `value`.
- **Collapsed-port route collision (BUG-2645).** The Cloud Run Admin v1
  instances IAM aliases could not mount because Memorystore Redis owned the same
  path shape. Requests now resolve by the `Host` a real client sends, and where
  one origin serves every service the AIP-136 custom method decides — Cloud Run
  owns exactly the three IAM verbs and Memorystore owns its five actions, so the
  sets are disjoint and the resolution is total rather than a fallback.
- **Worker-pool scaling (BUG-2646) stays open, correctly.** The fields are
  modelled and covered end to end, but fetching the newest live Discovery
  document (revision 20260713) showed it still declares only
  `manualInstanceCount` — as does the published REST reference — even though
  gcloud's own generated client and the GA Terraform provider send all four
  members. This is an upstream publication lag, not a simulator defect, so the
  six resulting `unknown-field` keys are allowlisted under that bug and the
  entry stays open until Google publishes them.

Every fix was proven non-vacuous by reverting it and watching the new tests fail
— including a real `terraform plan -detailed-exitcode` returning 2 with the
missing scaling block, and drift on all three Cloud Run resources without the
probe fields. Filed BUG-2649 (roughly seven AWS CLI tests skip when the
installed `aws` predates an operation — the deceptive shape the no-skip-if-absent
rule targets) and BUG-2650 (vendored specs across all three clouds have drifted
behind upstream while the freshness check only reports, so the conformance
ratchet's authority decays silently).


## 2026-07-26 — Closed the simulator test-contract gaps, uncovering 31 fidelity bugs

`scripts/check-simulator-tests.sh` only enforces *newly added* `Register`/
`HandleFunc` lines, so handlers written before the hook could carry no SDK, CLI,
or Terraform coverage at all. Pointing the real clients at those never-exercised
surfaces is what made this pass valuable: it surfaced 31 genuine fidelity bugs,
several of which were breaking real code paths.

- **AWS Cloud Map (`cloudmap.go`) — 17 bugs.** Tagging was entirely fake
  (`TagResource`/`UntagResource` discarded input, `ListTagsForResource` always
  returned empty), which broke the ECS backend's network-state recovery — it
  finds its namespace by the `sockerless:network-id` tag. `ListNamespaces`
  ignored `Filters`, so the same backend's `TYPE=DNS_PRIVATE` query matched
  everything. `GetOperation` fabricated `SUCCESS` for any unknown operation id
  (pure synthetic behaviour) and Register/DeregisterInstance returned operation
  ids that were never stored — the two defects concealed each other. Also fixed:
  a simulator-internal Docker network name leaking onto the Namespace wire
  shape, missing pagination on five list operations, `DiscoverInstances`
  ignoring custom health status, dropped `HealthCheckCustomConfig`/SOA TTL/
  `CreatorRequestId`, missing uniqueness and validation errors, wrong
  not-found error selection, and state left behind on delete. All 30 operations
  in the servicediscovery model now have SDK coverage, 30 have CLI coverage, and
  the Terraform stack gained tags, an instance resource, a public HTTP
  namespace, and three data sources.
- **Google Cloud Run (`cloudrunworkerpools.go`, `cloudruninstances.go`) — 13
  bugs.** `instances.patch` was not implemented at all despite being in the
  Discovery document. Nine dropped-field bugs (found by running a real
  `terraform apply` → `plan -detailed-exitcode` cycle and watching it never
  converge) restored `Volume.emptyDir`/`cloudSqlInstance`,
  `SecretVolumeSource.items`, `Container.workingDir`/`dependsOn`/`baseImageUri`,
  `ResourceRequirements.cpuIdle`/`startupCpuBoost`, `VolumeMount.subPath`,
  `VpcAccess.networkInterfaces`, and several worker-pool/instance top-level
  fields. IAM verbs on a nonexistent resource returned an empty policy instead
  of `NOT_FOUND`. Most consequentially, the spec-conformance test allow-listed
  the entire `/v2/` prefix as "Artifact Registry OCI data plane", silently
  exempting every Cloud Run v2, Cloud Functions v2, and Logging v2 route from
  Discovery conformance — narrowed to the five real OCI subtree mounts, with the
  suite still green, so those routes are now genuinely checked.
- **Azure — one real bug plus the BUG-2644 coverage.** `patchWebSite` set
  `HTTPSOnly` unconditionally, so a tags-only PATCH silently cleared it; it is
  now presence-aware (absent stays unchanged, explicit `false` still applies)
  and honours the previously-ignored `enabled`/`clientCertMode`. The ACR-registry
  and Container-Apps-job PATCH handlers — correct but untested — gained SDK and
  CLI coverage, as did the Container Apps environment `/storages` sub-resource,
  ten undriven Logic Apps clients, and the capital-`F` `serverFarms` routes.

An audit heuristic that keyed on file names overstated the gap: several files it
flagged as untested were already covered, and each agent verified the real state
before working. Filed BUG-2645 (a Cloud Run v1 instances IAM alias blocked by a
collapsed-port route collision with Memorystore Redis), BUG-2646 (worker-pool
scaling fields newer than the pinned Discovery revision), BUG-2647 (Cloud Run
container probes and `EnvVar.valueSource` unmodelled), and BUG-2648 (a Cloud Map
SDK-test client pinning `HostnameImmutable` to dodge the modeled `data-` host
prefix).


## 2026-07-26 — Simulator consoles reach full functional parity (one pass)

A single comprehensive pass brought all three consoles to full functional
parity with their real cloud consoles — completing CRUD (adding the Update verb),
lifecycle actions, and the complex compute-resource creation deferred through the
incremental passes. Every flow uses real cloud APIs over the existing federated
data plane (federation/broker/signing logic untouched — only `api.ts` functions
were added); no simulator operation needed adding (the audit confirmed every
update/action/create op already existed).

- **AWS (real Cloudscape)** — Update: Lambda config (memory/timeout/env),
  CloudWatch retention, S3 versioning + tags, ECR tag-mutability/scan-on-push,
  and a reusable tags editor via `TagResource`/`UntagResource`. Actions: Lambda
  Test (Invoke). Creates: Create Lambda function (container-image or Zip-from-S3)
  and ECS Run task (existing family or an inline task definition).
- **Google Cloud (hand-built Material)** — Update: GCS bucket (storage class +
  labels), Artifact Registry (labels), Cloud Function config, Cloud Run job
  config, via real `PATCH` (with a reusable `LabelsEditor`). Actions: Cloud Run
  job Run (`jobs.run` → a real execution). Creates: Create Cloud Run job and
  Create Cloud Function. Plus a nav-drawer product search for flyout parity with
  AWS. Long-running operations driven through real `operations.get` polls.
- **Microsoft Azure (real Fluent)** — Update: a reusable tags editor via ARM
  `PATCH` on ACR/Storage/Container Apps/Functions, plus Container App job config,
  Storage account SKU/access-tier, and ACR SKU/admin-user. Actions: Function App
  start/stop/restart; Container App job Run/Stop. Creates: Create Container App
  job (ensuring the resource group + managed environment first) and Create
  Function App (ensuring a Consumption plan + runtime).

Held the bar throughout: real design tokens, light and dark at WCAG AA, axe
zero-violations on every new dialog/form/action surface in both themes, the
federated data plane untouched, real APIs only with honest error surfacing and
no fakes. Verified: all three packages typecheck/knip/build clean; new vitest
covering request shaping + form behavior for every flow (AWS 67, GCP 103, Azure
86); the package Playwright suites green with axe both themes (AWS 108, GCP 91,
Azure 77); and the Shauth relying-party matrix now runs the complete
authenticated story across all three consoles — including create → update (ECR
scan config; ACR admin) and compute-create → run (a Cloud Run job creating a
real execution). Filed BUG-2644 (an existing Azure ACR/Container-Apps PATCH
test-contract gap the work surfaced).

All three consoles are now faithful in look (real design systems on AWS/Azure,
faithful Material on GCP) and functionality (Create, Read, Update, Delete plus
lifecycle actions and compute-resource creation) against real cloud APIs.


## 2026-07-26 — Google Cloud and Azure console resource-deletion flows

Completed the consoles' create/read/delete parity: the AWS console already
deleted every resource, but the Google Cloud and Azure consoles could only
delete admin resources (projects/service accounts; app registrations/
subscriptions) — their compute and storage pages had no delete, which the real
consoles all have. Added delete (a list-page multi-select action and a
detail-page action, each opening a confirm surface that names the resource and
warns the action is irreversible) to:

- **Google Cloud** — Cloud Storage buckets (`storage.buckets.delete`, real 204),
  Artifact Registry repositories, Cloud Functions, and Cloud Run jobs (each a
  long-running operation driven through the real `operations.get` poll, reusing
  the create flow's machinery — a new `waitV2Operation` for the `/v2` functions/
  jobs collection). Fixed a real bug the delete surfaced: `authorizedJSONDelete`
  always called `response.json()`, which throws on the 204 No-Content body a
  bucket delete returns — it now returns undefined for 204 while still throwing
  the real error body on failure.
- **Microsoft Azure** — Container registries, Storage accounts, Container Apps
  jobs, and Function Apps via real ARM `DELETE` (all synchronous — the handlers
  return 200 when the resource existed or 204 when already gone, so no LRO
  polling). A shared real-Fluent `AzureConfirmDialog` backs every confirm.

Both follow the AWS delete template, preserve the federated bearer/broker/
endpoint logic (only delete functions added to api.ts), and hold the bar: light
and dark at WCAG AA, axe zero-violations on the confirm surfaces, existing tests
intact.

Verified: both packages typecheck/knip/build clean; new vitest (delete request
shaping incl. the 204-as-success handling, the LRO poll loop, and error
surfacing; plus full select→confirm→delete→invalidate round trips against a
mocked federated transport); the GCP (77) and Azure (67) package Playwright
suites green; and the Shauth relying-party matrix now runs a full create →
delete → gone round trip as the signed-in operator for a Cloud Storage bucket
and a Container registry through the real APIs, proving the authenticated delete
end to end. All three consoles now have real, end-to-end-proven Create, Read,
and Delete for these resources (Update remains the deferred piece).


## 2026-07-26 — Google Cloud and Azure console resource-creation flows

Extended the resource-creation parity started on the AWS console to the other
two, so all three consoles can create their simple resources (not just list and
inspect them):

- **Google Cloud** — the "Create bucket" (Cloud Storage) and "Create
  repository" (Artifact Registry) buttons had been disabled placeholders; they
  now open a Material `GcpDialog` wired to real `storage.buckets.insert`
  (`POST /storage/v1/b`) and `projects.locations.repositories.create`
  (`POST …/repositories`), the latter driven through a real `operations.get`
  long-running-operation poll loop the way a real client does — not an
  assume-done shortcut. GCP's wire helpers also gained a `GcpApiError` that
  parses Google's real `{"error":{code,message,status}}` body, so conflicts
  surface the real service message.
- **Microsoft Azure** — Storage accounts and Container registries gained a
  Fluent create form wired to real ARM PUTs (each idempotently ensuring the
  resource group first, as a real client does), settled synchronously the way
  the simulator's storage/ACR handlers return them (200 / provisioningState
  Succeeded). Errors surface ARM's own `error.message`.

Both follow the AWS create-flow template (a Create control opening the form,
`useMutation` over the federated path, invalidate-on-success so the resource
appears), preserve the federated bearer/broker/endpoint logic, and hold the
bar: light and dark at WCAG AA, axe zero-violations on the open create surfaces
in both themes, existing tests intact.

Verified: both packages typecheck/knip/build clean; new vitest for the request
shaping + form behavior + conflict surfacing; the GCP (60) and Azure (59)
package Playwright suites green; and the Shauth relying-party matrix now creates
a Cloud Storage bucket and a Container registry as the signed-in operator
through the real APIs and sees each appear in the list — the authenticated
create → list-refresh round trip the per-package suites cannot prove (no
identity provider) — alongside the AWS ECR-repository create proof. All three
consoles now have real, end-to-end-proven resource creation for these services.


## 2026-07-26 — AWS console resource-creation flows

Functional-parity pass: the AWS console could list and inspect S3 buckets, ECR
repositories, and CloudWatch log groups but not create them (unlike IAM and
Organizations, which already had create flows, and unlike the real AWS console).
Added a create flow to each, built with the real Cloudscape components the
console now runs on:

- **S3 — Create bucket**: `PUT /{bucket}` (CreateBucket, REST-XML) via a new
  `createS3Bucket`, with DNS-name validation and `BucketAlreadyOwnedByYou`
  surfaced.
- **ECR — Create repository**: `CreateRepository` (awsjson1.1) via
  `createECRRepository`, `RepositoryAlreadyExistsException` surfaced.
- **CloudWatch Logs — Create log group**: `CreateLogGroup` (awsjson1.1) via
  `createCWLogGroup`, `ResourceAlreadyExistsException` surfaced.

Each is a primary "Create …" button in the table header actions opening a
Cloudscape `Modal`/`FormField`/`Input`, `useMutation` over the federated SigV4
path (a new `awsRestXmlPut` helper joined the existing rest/json helpers;
`federation.ts`/`sigv4.ts` signing logic untouched), invalidating the list on
success so the new resource appears. Matches the existing IAM/Organizations
create-modal template exactly (focus trap, error `Alert`, testids).

Verified: typecheck/knip clean; vitest (6 new tests — a success and an
error-surfacing path per resource against a mocked federated transport);
Playwright 97/97 with axe zero-violations on all three create modals in both
themes; and the Shauth relying-party matrix green — it now creates an ECR
repository as the signed-in operator through the real CreateRepository API and
sees it appear in the list, proving the authenticated create → list-refresh
round trip end to end (the per-package e2e can't, having no identity provider —
its unsigned writes are correctly 403'd by the simulator's IAM enforcement).

The GCS-bucket / Artifact-Registry-repo (Google Cloud) and storage-account / ACR
(Azure) equivalents are the natural follow-ups.


## 2026-07-26 — Azure portal adopts the real Fluent component library

Mirroring the AWS console's Cloudscape migration, the Azure simulator portal
(ui/packages/simulator-azure) moved from its hand-built Fluent *approximation*
to the real `@fluentui/react-components` (Fluent UI v9 / Fluent 2 — the system
the real Azure portal is built on). React 19 compatibility was spike-verified
first (Fluent's peer range explicitly includes React 19).

The whole portal renders through genuine Fluent now: a `FluentProvider` whose
theme switches between light and dark via the shared `useTheme` hook, plus
`Toolbar`, `Popover` (the header Cloud Shell/Notifications/Help disclosures),
`Breadcrumb`, `Badge`, `Accordion` (Essentials + service-menu groups), `Table`
(all list and sub-resource tables, hand-composed from `TableRow`/
`TableSelectionCell` to keep the accessible per-row selection names), `Field`/
`Input`/`Select`/`Button` (every form), `MessageBar`, `Spinner`, and real Fluent
System Icons (`@fluentui/react-icons`) replacing the hand-drawn SVGs. The
server-side federation broker + ARM/Graph data plane (federation.ts/api.ts) were
untouched — only rendering changed.

The portal's signature header blue is the iconic Azure `#0078d4` (and its
classic hover/pressed shades) in both themes, applied by overriding Fluent's
brand-background tokens on the theme rather than accepting Fluent's stock brand
`#0f6cbd` — so the migration to real Fluent did not cost the one most
recognizable Azure colour. Light and dark both render at WCAG AA.

The migration also fixed real defects it surfaced: a `Spinner` with no
accessible name (caught by axe) and the focus-indicator expectations updated to
Fluent's real mechanism (`data-fui-focus-visible` + underline, since Fluent
zeroes the outline). A jsdom test-environment race (Fluent's tabster scheduling
a MutationObserver after teardown) was fixed with `afterEach(cleanup)` plus a
defensive `NodeFilter` polyfill in the vitest setup.

Verified end to end: typecheck/knip clean; 34 vitest; 53 Playwright (axe
zero-violations, both themes, across list/detail/not-supported/popover
surfaces); the Shauth relying-party matrix green — the Azure federated flow
(sign-in → federation broker → Entra client-secret minting → the authenticated
Container Apps job detail render) works through the real Fluent DOM, with every
RPS-critical data-testid preserved so the matrix needed no changes.

Bundle delta: dist ~384 KB → ~793 KB (112→227 KB gzip), ~2.1x — the real cost of
Fluent's Griffel CSS-in-JS runtime, tabster, and the `@fluentui/react-*`
subpackages, proportionally smaller than the AWS Cloudscape jump (~4.3x). Both
the AWS (Cloudscape) and Azure (Fluent) consoles now run on their real design
systems; Google Cloud stays hand-built (no official Google console component
library).


## 2026-07-26 — AWS console adopts the real Cloudscape component library

The AWS simulator console (ui/packages/simulator-aws) moved from its hand-built
Cloudscape *approximation* to the real `@cloudscape-design/components` — the
system the AWS Management Console itself is built on — the biggest single step
toward literal AWS console parity. React 19 compatibility was spike-verified
first (real Table/Tabs/Button render and behave under the stack).

The whole console renders through genuine Cloudscape now: `AppLayout`,
`TopNavigation`, `BreadcrumbGroup`, `Table` (with `TextFilter`/`Pagination`),
`Header`, `Modal`, `Tabs`, `Button`, `Link`, `Badge`, `StatusIndicator`,
`Alert`, `KeyValuePairs`, `ColumnLayout`, `CopyToClipboard`, `Input`,
`FormField`, `Spinner` — across the shell, all seven list pages, all five
detail pages, every modal and tab strip. Light and dark are Cloudscape's own
WCAG-AA modes via `applyMode`, wired to the existing `useTheme` hook, so
`tokens.css`/`console.css` shrank from ~600 to ~60 lines (only the always-dark
header account cluster beside `TopNavigation` and the CloudWatch Logs
transcript viewer stay hand-built). Two composites stayed hand-built for a real
reason: the searchable multi-column Services flyout (no Cloudscape equivalent)
and the side navigation (the packaged `SideNavigation` badge can't carry a
distinct "not supported" accessible name). The federated SigV4 data plane
(`federation.ts`/`api.ts`/`sigv4.ts`) was untouched — only rendering changed.

The migration also fixed real defects it surfaced: a duplicate `<main>`
landmark, an unlandmarked header account cluster, a Modal close button with no
accessible name, a dark-mode contrast failure in the Services panel, and a
duplicate not-supported aria-label.

Verified end to end: typecheck/knip clean; 49 vitest; 85 Playwright; axe-core's
full ruleset zero-violations across every page, the not-supported page, the
Services flyout, and a dialog in both themes; and the Shauth relying-party
matrix green — the AWS federated flow (sign-in → SigV4 reads → IAM key minting
→ Organizations account creation) works through the real Cloudscape DOM (the
matrix's account-row lookup was adapted to find the row by its Cloudscape link).

Trade-off, recorded honestly: the `dist` bundle grew from ~460 KB to ~2.0 MB
(JS 111→311 KB gzip, CSS 7→226 KB gzip) — the real cost of a full design system
versus a hand-rolled approximation. Scoped to AWS deliberately: Cloudscape is
open-source and the most verifiable; Microsoft Fluent (Azure) can follow, and
Google Cloud stays hand-built (no official Google console component library).


## 2026-07-25 — Simulator console parity pass 3: services flyout, ACR coordinate, authenticated detail render

Pass 3 closed the loose ends parity passes 1 and 2 left open, holding the same
bar (real design tokens, light and dark at WCAG AA, axe-clean ARIA).

- **AWS "All services" mega-menu flyout.** The real console's Services button
  now opens a full-width overlay with a live search field and the service
  catalogue in grouped columns — reusing the single `serviceCatalog.ts` (one
  supported/"Not supported" rule, applied in both the side nav and the flyout),
  with a focus trap, Escape/outside-click dismiss, and measured light/dark
  contrast. The left side nav stays the current-section affordance.
- **BUG-2643 fixed — ACR loginServer coordinate.** `simulators/azure/acr.go`
  hardcoded `loginServer` to `<name>.azurecr.io`, which no browser could reach;
  it now derives the host from the request via `azureACRLoginServer(r, name)`
  like Storage/Key Vault, so the portal's ACR detail blade resolves
  repositories/tags against the simulator. The ACA/Azure Functions overlay
  push/pull is unaffected (it uses the `SOCKERLESS_AZURE_ACR_*` coordinates, not
  `loginServer`).
- **Authenticated end-to-end detail render.** The relying-party matrix seeds a
  Container Apps managed environment and job through the real Azure Resource
  Manager API, then — after the operator signs into the portal through Shauth —
  opens that job's detail blade and asserts its live Essentials render (the
  resource group parsed from the resource id, the provisioning state the
  simulator assigned) over the federated ARM path. This closes the gap the
  earlier passes noted: detail pages were component- and structurally-tested,
  and are now proven rendering live cloud data in a real browser end to end.


## 2026-07-25 — Simulator console parity pass 2: resource detail views

Pass 2 built the resource-detail functionality pass 1 deferred (pass 1 dropped
the "View details" affordance because no detail pages existed), holding pass 1's
bar throughout — real design-system tokens, light and dark at WCAG AA with
contrast measured on painted surfaces, axe-clean ARIA, real API wiring.

- **AWS**: real API-wired detail pages for all five supported services — ECS
  task (DescribeTasks + DescribeTaskDefinition, tabbed Containers/Network/Task
  definition), Lambda function (GetFunction), ECR repository (DescribeImages),
  S3 bucket (ListObjectsV2 + GetBucketLocation), CloudWatch log group
  (DescribeLogStreams + GetLogEvents) — each with a Cloudscape details-with-tabs
  layout, "View details" restored through the table's actions render prop, and a
  new WAI-ARIA `AwsTabs` component (roving tabindex, arrow/Home/End). The
  "All services" mega-menu flyout stayed deferred (a distinct interactive
  surface), the static grouped sidebar kept.
- **Azure**: real ARM/data-plane detail blades for Container App jobs
  (executions + start/stop), Function Apps (app settings + functions), Container
  registries (repositories/tags via minted admin credentials against the ACR
  data plane), and Storage accounts (containers/blobs via a minted account SAS,
  parsing the real EnumerationResults XML) — each an Essentials grid + command
  bar + sub-resource tables. The global header gained Cloud Shell, Notifications,
  and Help as honest, accessible popover affordances (W3C ARIA APG
  menu-button/dialog pattern). Filing surfaced BUG-2643: ACR's `loginServer` is
  hardcoded to `<name>.azurecr.io` rather than derived from the request host, so
  the blade's repositories/tags panel shows a loud honest error until that
  simulator coordinate is fixed.
- **Google Cloud**: deepened the existing detail pages toward the real console
  — Cloud Run job (Details/Executions tabs with a real per-execution status),
  Artifact Registry repo (an Images tab wired to the previously-unused
  dockerImages.list), GCS bucket (an Objects tab), Cloud Function (surfaced the
  serviceConfig fields) — plus a real Cloud Logging query bar (query-language +
  minimum-severity composing the server-side filter) with entry expansion, and
  closed a gap: simulator-gcp was the only console package missing
  @axe-core/playwright, added here (which caught a pre-existing invalid-ARIA
  avatar).

Verification: all three packages typecheck / vitest / build / knip green;
Playwright e2e per package (AWS 59, GCP 50, Azure 53) covering the new
surfaces' structure, measured light/dark contrast, and axe (zero violations);
the shells were rendered in a browser in both themes and measured. Detail
pages' data-bound rendering is proven at the component level (vitest with
realistic props) and structurally in e2e; the authenticated end-to-end detail
render (which needs the federation token) remains component/structural, as with
pass 1's detail pages.


## 2026-07-25 — Simulator console parity pass 1: faithful shells, not-supported pills, light/dark, a11y

A first parity pass raising all three simulator consoles toward their real
cloud consoles' design languages, grounded in the published design systems
(not memory — the ground truth came from the real token sources), with the
rendered output verified in a browser in both light and dark.

- **AWS console → Cloudscape.** Token values read directly from the real
  `@cloudscape-design/design-tokens` package (light and dark), correcting a
  wrong link colour from a prior attempt (`#0972d3` → the real `#006ce0` /
  `#42b4ff` dark). A faithful service navigation lists nine of AWS's real
  "All services" groups; the seven supported services link to their pages and
  ~22 commonly-expected unsupported ones (EC2, RDS, EKS, VPC, …) carry an
  accessible "Not supported" pill and route to an honest not-implemented page.
- **Google Cloud console → Material 3.** A Material navigation drawer lists
  eleven real product groups with the supported/unsupported split (~25 not-
  supported chips); the theme moved onto the shared Light/Dark/Same-as-device
  hook (it previously neither persisted nor honoured the system preference);
  dialogs gained Escape/scrim/focus-return.
- **Azure portal → Fluent 2.** Token values read from the real `@fluentui/tokens`
  source, replacing Fabric-era neutrals with true Fluent 2 neutrals; the header
  blue was already Fluent's `colorBrandBackground` `#0078d4`. A service menu
  lists ten real groups (~16 not-supported badges).

Across all three: light and dark both render correctly with WCAG 2.1 AA
contrast measured against the actually-painted surfaces in a real browser
(every sampled text pair exceeded AA in both modes — AWS 17.3/11.1, GCP on
`#f0f5fe`/`#202124`, Azure 15.5/14.6); landmark roles, `aria-current`, focus
traps on dialogs, and keyboard operability were added or verified; the
not-supported affordance is conveyed non-visually (an explicit link aria-label
"<service>, not supported in this simulator" on every cloud, never colour
alone); and each package's Playwright suite gained not-supported, light/dark,
contrast, and axe-core (zero-violation) coverage. All real API wiring
(federated reads/mutations, the project picker, the Azure federation broker)
was preserved untouched.

This is an honest **pass 1**, not literal 100% parity: the shells are
hand-built approximations of the real component libraries (not the vendored
`@cloudscape-design/components` / `@fluentui/react-components`), icons are
hand-drawn in each system's style rather than the proprietary icon sets, and
per-product colour logos and some header affordances (Azure Cloud Shell/
notifications) are deliberately not replicated. Group ordering and the exact
service catalogue were built from each console's public IA, not authenticated
screenshots.


## 2026-07-25 — Azure `client_credentials` rejects unregistered clients

BUG-2639, the last actionable fidelity gap from the Console Self-Service
roadmap: the Azure simulator's v2.0 `client_credentials` grant minted a token
for any client id — an unregistered id fell through to an implicit-client
branch with no secret validation, where real Microsoft Entra rejects unknown
clients with `unauthorized_client` AADSTS700016. Enumerating every
`client_credentials`-with-secret call site showed the harnesses had almost all
already converged on one coordinate (`test-client-id`/`test-client-secret`), so
the fix was a clean single-coordinate consolidation, not a mass migration: the
simulator seeds a well-known bootstrap Entra application (that appId, a hashed
secret password credential, and its service principal — the Azure analog of the
AWS `test`/`test` bootstrap), the few stragglers (a couple of test subtests, and
a relying-party harness call that had relied on the implicit grant with no
secret at all) were pointed at it, and the implicit-client branch was deleted so
an unregistered client id now returns the real AADSTS700016. A new SDK test
asserts the rejection; the Azure Container Apps and Azure Functions backends
were confirmed unaffected (they authenticate through the managed-identity
`/msi/token` endpoint, never `client_credentials`).


## 2026-07-25 — Closed the console/simulator fidelity follow-ups the roadmap surfaced

Three fidelity gaps filed during the Console Self-Service phases, resolved
together:

- **AWS console (BUG-2637)**: `AwsTable`'s default "View details"/"Delete"
  header actions rendered enabled but did nothing on the Amazon ECS, AWS
  Lambda, Amazon ECR, Amazon S3, and CloudWatch Logs pages. Each page now
  passes the `actions` render prop so the inert defaults no longer render:
  "View details" was dropped (these resources have no detail view), and a real
  Stop/Delete was wired over the federated Signature Version 4 path (ECS
  StopTask; Lambda, ECR, S3, and CloudWatch Logs deletes) with a confirm
  dialog and the real cloud error surfaced.
- **Google Cloud simulator (BUG-2638)**: `serviceAccounts.create` silently
  overwrote an existing service account; it now returns Google Cloud IAM's real
  409 `ALREADY_EXISTS`. The Cloud Run and Cloud Run Functions backend harnesses
  that re-provision `sockerless-runner` against a persistent simulator moved to
  get-or-create, and SDK + CLI tests cover the conflict.
- **AWS simulator (BUG-2642)**, found by the Boy Scout check while fixing the
  console actions: AWS Lambda's REST API surface bypassed SigV4/IAM enforcement
  entirely — an unauthenticated call returned data, a hole in the credential
  enforcement contract. A `lambdaEnforced` wrapper (mirroring `s3Enforced`) now
  verifies the Signature Version 4 signature and evaluates the real `lambda:`
  IAM action on every control-plane route, returning Lambda's REST-JSON auth
  error shape; the `/2018-06-01/runtime/...` container-polling routes stay
  unenforced, so function execution is unaffected — proven by the invoke suite
  plus a new unsigned/wrong-secret deny test.

BUG-2639 (the Azure v2.0 implicit grant for unregistered client ids) stayed
open as a deliberate interim state: removing it is a mass migration of every
Azure test harness to provisioned app registrations, better done as its own
considered change.


## 2026-07-25 — Deployment recipe, and the Azure portal federates for real

Phase 4 of the Console Self-Service roadmap: the deployment and provisioning
recipe, and the Azure federation deployability fix (BUG-2640) it carries.

A committed `deploy/` hosts Sockerless Admin, the three simulators, and a
Shauth stack (Shauth + Ory Hydra + PostgreSQL) at persistent origins:
`compose.yaml` runs the published `ghcr.io/e6qu/*` images (a required
`SOCKERLESS_TAG`, no implicit latest), `compose.build.yaml` builds from source,
`provision.sh` registers every console as a Shauth OpenID Connect client and
provisions the cloud federation resources through the real APIs (AWS IAM OIDC
provider + roles via SigV4, GCP workforce pool providers, Azure managed
identity + federated identity credentials, capturing the generated Azure client
id into `.env.generated`), and `smoke.sh` proves every health endpoint, the
unauthenticated console redirects, each data plane's reject-unauthenticated /
answer-authenticated contract, and a `sockerless login` authorize URL. A
load-bearing discovery shaped the recipe: Admin and the consoles reject any
non-HTTPS, non-`localhost` OpenID Connect issuer, so a Caddy reverse proxy
TLS-terminates every persistent hostname on one port with its own local CA,
trusted by the backchannel-logout and federated-JWT-verifying services through
`SSL_CERT_FILE` (compose-only, no image change). The full boot → provision →
smoke passed fresh and idempotent; no CI job was added because a cold
from-source build of the five images alone runs ~10 minutes, past the
15-minute ceiling before provisioning starts, so `make deploy-smoke` is the
documented manual gate.

BUG-2640 closed: the Azure portal's Workload Identity Federation exchange moved
into the console's own **server-side broker** (`/auth/federation/token`, using
the ui-auth session's assertion) — real Microsoft Entra serves no CORS for the
`client_credentials` grant, so the browser could never read the token
response, which is why the browser-side exchange only ever worked co-served.
The Azure simulator gained faithful Azure Resource Manager and Microsoft Graph
CORS (the Entra token endpoint deliberately gets none — the reason the broker
exists), the SPA now calls the broker on its own origin and reads the cloud
cross-origin over that CORS, and the relying-party harness runs the Azure
console and cloud as **separate processes**: it provisions the console's
managed identity and federated identity credential on the cloud process, then
starts the console pointing every coordinate at it. That unblocked the Azure
browser data plane and the app-registration / client-secret **minting flow
deferred since the credential-minting phase** — the relying-party matrix now
mints an Entra client secret through the portal in a real browser, alongside
the AWS access key and Google Cloud service-account key.

## 2026-07-25 — `sockerless login` signs the terminal into every cloud

Phase 3 of the Console Self-Service roadmap: the packaged terminal analog of
`aws configure sso` / `gcloud auth login` / `az login`. `sockerless login`
(zero-dependency, stdlib-only in `cmd/sockerless`) runs the RFC 8252
native-app flow — ephemeral loopback listener, S256 PKCE, the authorize URL
printed (and opened unless `--no-browser`), Shauth sign-in and the one-time
consent screen in the operator's browser — then wires **vendor-native
credentials** that the vendor tools refresh themselves, never one-shot copied
secrets:

- **AWS**: an INI-preserving `~/.aws/config` profile with `role_arn`,
  `web_identity_token_file`, `region`, and `endpoint_url` — the AWS CLI runs
  `AssumeRoleWithWebIdentity` itself (`aws --profile sockerless-<ctx> sts
  get-caller-identity` returns the assumed federation role).
- **Google Cloud**: a real workforce `external_account` Application Default
  Credentials file plus a dedicated gcloud configuration with the proven
  `api_endpoint_overrides`, activated via `gcloud auth login --cred-file`.
  Proving this surfaced BUG-2641: gcloud resolves the signed-in account via
  STS token introspection (`POST /v1/introspect`, RFC 7662), which the
  simulator lacked — implemented against gcloud's captured live wire (HTTP
  Basic with Google's published gcloud OAuth client, `principal://…/subject/…`
  username, `active:false` for unknown tokens as real Google answers) with
  SDK-shaped and real-gcloud CLI coverage.
- **Microsoft Azure**: `az cloud register` + `az login --service-principal
  --federated-token` — az stores the assertion and re-exchanges it on demand.
  az/MSAL reject any http authority, so the relying-party harness runs a
  second TLS-serving Azure simulator instance for the CLI's coordinates.

Shauth findings baked into the harness: the CLI registers as a public Hydra
client (`token_endpoint_auth_method: none`) with the RFC 8252 loopback
any-port redirect; non-managed clients traverse Shauth's explicit consent
screen once. `sockerless logout` removes the token, the ADC file, and the
CLI's own profile section, and runs `az logout`. The relying-party matrix
drives the whole story: spawn the CLI, sign in and authorize in a real
browser, then prove `aws`, `az`, and `gcloud` each work vendor-natively
against the simulators, then log out.

## 2026-07-25 — Consoles manage accounts, projects, and subscriptions

Phase 2 of the Console Self-Service roadmap: a Shauth-authenticated operator
manages the account containers themselves, through each cloud's real APIs over
the federated session.

- **Google Cloud** gained a real Cloud Resource Manager slice — and building
  it surfaced that the existing partial v3 surface was faked: the sim
  synthesized an ACTIVE project for any never-seen ID, returned a synthetic
  done-operation for any operation name, never enforced duplicate-ID 409s, and
  used the wrong v3 `name` form. All replaced with the real contract, verified
  against what the real clients actually speak (gcloud's CRM v1 with its
  `lifecycleState:ACTIVE` filter; Terraform's v1 lifecycle plus an
  unconditional Cloud Billing read honoring `cloud_billing_custom_endpoint`;
  the v3 GAPIC client whose proto resolution rejects invented operation
  metadata types — each operation now carries its verb's real metadata
  message). v1 create/list/update/delete/undelete/operations plus Cloud
  Billing `getBillingInfo` were added, projects resolve by ID or number,
  unknown projects answer 403 `PERMISSION_DENIED` without disclosing
  existence, and delete is the real 30-day soft-delete. The console gained the
  real header project-picker chip (search, New Project driving the create
  LRO, Manage resources page with Shut down) and every console page now reads
  the selected project — the hardcoded project constant is gone. SDK, gcloud
  CLI, and `google_project` Terraform coverage landed in the same change,
  including a real-provider apply/plan-idempotency/destroy proof.
- **Microsoft Azure** gained the Microsoft.Subscription alias API at 2021-10-01
  Swagger fidelity (vendored): alias PUT (billing-scope creation and
  subscription adoption), the documented provisioning-state polling model
  (verified against both azcore's body poller and go-autorest's), rename/
  cancel/enable, and created subscriptions backing `GET /subscriptions`. The
  portal gained a Subscriptions blade (list, Add with live provisioning,
  detail with Cancel/Reactivate — no invented delete; Azure has none).
  armsubscription SDK, az CLI (`az rest` on the documented wire — the alias
  commands live in a preview extension), and `azurerm_subscription` Terraform
  coverage (both modes) landed together; the subscription resources run as
  their own `tf (azure subscription)` CI shard because the provider's fixed
  60-second settle delays don't fit the shared azure stack's budget.
- **AWS** gained the Organizations console page — accounts table, the real
  console's asynchronous "Add an AWS account" flow (`CreateAccount` polled via
  `DescribeCreateAccountStatus`), the organization-not-in-use state with
  "Create an organization", remove/close actions, and account detail — over
  the existing Organizations slice. `awsJson` now surfaces the real awsjson1.1
  error code so pages branch on the service error, and the relying-party
  federation role gained `organizations:*`.

The Shauth relying-party matrix drives both new browser flows (create an
organization account to SUCCEEDED; create a project through the picker and
switch to it) and passed with its exit code observed directly.

## 2026-07-24 — Consoles mint real CLI credentials for Shauth-authenticated users

Phase 1 of the Console Self-Service roadmap: each simulator console gained its
cloud's real credential pages, driven by the operator's federated credentials
calling the cloud's real APIs — never a console-only endpoint.

- **AWS**: an IAM Users page and a per-user Security-credentials page (AWS
  Identity and Access Management `CreateUser`/`CreateAccessKey`/`ListAccessKeys`
  /`DeleteAccessKey`/`UpdateAccessKey` over the AWS Query protocol, SigV4-signed
  browser-side with the federated temporary credentials). "Create access key"
  reproduced the real console's one-time disclosure — the secret is viewable
  exactly once, masked behind Show/Hide, with the exact `aws configure` values
  and an endpoint-scoped `aws sts get-caller-identity` verification command. A
  CLI test proved the loop: a key minted via `aws iam create-access-key`
  authenticated `aws sts get-caller-identity` (returning the minted user's ARN)
  and a wrong secret failed with `SignatureDoesNotMatch`. The header/Overview
  Region badges now render the same Region every SigV4 signature scopes to.
- **Google Cloud**: Service Accounts list/create/delete and a Keys tab
  (`serviceAccounts`/`keys` IAM APIs over the federated bearer) with the real
  console's one-time JSON key download (`privateKeyData` decoded to
  `<project>-<keyid>.json`, unrecoverable afterwards) and a gcloud usage panel
  proven verbatim by a CLI test (`gcloud auth activate-service-account` with a
  minted key authenticates; a tampered key fails with `invalid_grant`). Fixing
  the end-to-end loop surfaced that the simulator's token endpoint **never
  verified assertion signatures** and discarded every minted key's public half:
  keys.create now registers the public key, deletion revokes it, and the OAuth
  2.0 token endpoint verifies the RS256 signature, expiry, and account state
  exactly as Google does — the backend harnesses' self-keypair helpers were
  deleted and moved to the real mint flow.
- **Microsoft Azure**: App registrations and Certificates & secrets blades
  (Microsoft Graph `applications`/`servicePrincipals` routes with a
  Graph-scoped federated token). The real portal mints secrets on the
  application object, so the simulator gained faithful Graph
  `applications/{id}/addPassword`/`removePassword` (secretText returned exactly
  once, SHA-256 verifier stored). Tracing validation found the v2.0 token
  endpoint **checked no client secret at all**; `client_credentials` for
  directory-registered applications now validates the secret and returns real
  AADSTS error shapes, proven by SDK and az-CLI tests (mint → ARM read; wrong
  and revoked secrets rejected). The sockerless-invented `/sim/v1/entra/users`
  routes and the `entraActiveOID` global were deleted; consumers migrated to
  Graph `POST /v1.0/users` provisioning and `login_hint` binding.

The Shauth relying-party browser matrix gained minting flows for the AWS and
Google Cloud consoles: the signed-in operator drives the real UI to mint a
credential over the federated session, and the one-time disclosure semantics
are asserted (secret gone after dismissal). Getting those flows green surfaced
two environment defects the suite had been silently missing: the harness's
console federation role lacked `iam:*` (the simulator's IAM enforcement
correctly denied the minting pages, exactly as real AWS denies an operator
role never authorized for the IAM console), and full-page navigations in the
new flows aborted the prior page's in-flight reads (the flows now navigate
through the console's own navigation, as an operator does). The Azure portal
got no browser-driven minting flow: its browser-side Workload Identity
Federation exchange is same-origin-only (real Microsoft Entra serves no CORS
for `client_credentials`), and the relying-party environment cannot provision
the portal's managed identity before console start — filed as BUG-2640 with
the deployment-phase fix shape (server-side federation broker + faithful CORS
+ separate console/cloud processes); Entra minting itself is proven end to end
by the Azure SDK and az CLI suites. BUG-2637 (inert default table actions),
BUG-2638 (`serviceAccounts.create` overwrite instead of 409), and BUG-2639
(implicit grant for unregistered client ids) were filed with fix shapes.

The `sim (aws sdk)` job — chronically within two minutes of the enforced
15-minute ceiling — hit it on runner variance and was split into four shards
(`compute` = `^Test[E]`, `data` = `^Test[D]`, `services-a-m` = `^Test[A-CF-M]`,
`services-n-z` = `^Test[N-Z]`),
mirroring the AWS CLI shards: shard regexes use the character-class form so
each suite's coverage gate reads only its own shard set,
`scripts/check-sdk-shard-coverage.sh` (pre-commit) asserts all 1152 SDK tests
match exactly one shard, the DynamoDB Local oracle pull rides the data shard
and the module unit tests the services-n-z shard, and
`.github/required-status-checks.txt` carries the four new contexts (branch
protection must swap `sim (aws sdk)` for them when this merges).

## 2026-07-24 — Closed the skip-if-absent gate hole and swept the last tool-absent skips

`scripts/check-no-tool-absent-skips.sh` only rejected `t.Skip`/`t.Skipf` lines, so
a TestMain-level `exec.LookPath(tool)` → `fmt.Println("… not found, skipping")` →
`os.Exit(0)` evaded it silently — which is how the Google Cloud and Azure CLI
suites came to skip themselves whenever `gcloud`/`az` were absent. The gate now
also rejects (2) a `fmt.Print*`/`log.Print*` line carrying a tool-absent phrase
and (3) a bare `os.Exit(0)` within a few lines of a `LookPath(` in the same hunk,
and it exempts skips that self-identify as platform/kernel-capability gates
(`runtime.GOOS`, `CAP_NET_ADMIN`, "requires a Linux host") so a legitimate GOOS
gate whose message happens to say "not available" is not a false positive.

Every remaining tool-absent skip was resolved to install-or-fail-loud:
- The Google Cloud CLI suite installs a pinned Google Cloud CLI release into a
  temp dir when `gcloud` is absent (mirroring the AWS suite's
  `installLatestAWSCLI`); the Azure CLI suite fails loud with an actionable
  message — the `az` Python application has no clean cross-platform TestMain
  install — each replacing its `os.Exit(0)` skip.
- The six `t.Skip("docker CLI not available")` guards in the AWS ECS VPC-networking
  CLI tests and the one in the Azure Cosmos differential test were vestigial:
  their TestMains already `docker build` images and `log.Fatalf` without Docker,
  so Docker is guaranteed present before any test runs. They were removed; the
  Linux + CAP_NET_ADMIN netns capability gates stayed.
- The `session-manager-plugin` (AWS ECS execute-command), `git` (AWS Amplify),
  `gcloud` (Google Cloud Firestore differential), and `nsenter` (realexec
  external-namespace round-trip) skips became `t.Fatal`. Each is a tool the
  relevant CI job already provides, so CI stayed green while a local run without
  it now fails loud with an actionable message instead of skipping unseen.

## 2026-07-24 — Gated required-check drift so a job rename can't stall the merge queue

Splitting the AWS CLI groups into shards once renamed their jobs while `main`'s
required-status-check list still demanded the old contexts, which could never
report again — so every pull request stalled as pending (BUG-2633). The list was
corrected then, but nothing prevented a recurrence.

`.github/required-status-checks.txt` is now a version-controlled manifest of the
required contexts, and `scripts/check-required-status-checks.sh` — wired into
pre-commit and the `build-gates` CI job — enumerates every check name any
workflow in `.github/workflows/*.yml` can emit, rendering each job's `name:`
template over its matrix (handling the inline-list, block-list, and `include:`
matrix forms), and fails when a required context is no longer emittable. So a
matrix job rename now fails the pull request that causes it, with the manifest as
the reviewable bridge to the branch-protection update. A maintainer-run
`--verify-branch-protection` mode reconciles the manifest against live branch
protection (failing loudly rather than skipping when admin credentials are
absent). The manifest matched all 39 current required contexts, and a negative
test confirmed the gate flags a renamed shard.

## 2026-07-23 — Made the simulators verify credentials, and fixed the ECS harness

The simulators accepted unverified caller-controlled credentials (BUG-2625, P0):
AWS derived identity from the cleartext `Credential=` access-key id and never
checked the SigV4 `Signature`; Google Cloud and Azure trusted arbitrary bearer
content (GCP had no data-plane auth at all; Azure verified a bearer only on its
UserInfo endpoint). All three now verify the way the real clouds do.

- **AWS** recomputes the SigV4 signature (canonical request, key derivation,
  constant-time compare) at the awsjson/query control plane and S3, looking up
  the secret for the presented long-term (`AKIA`) or temporary (`ASIA`) key —
  temporary-credential secrets and session tokens are now persisted, and a
  bootstrap account credential (`test`/`test`, the coordinate every client
  already signs with) is seeded so an account can act before it mints its own
  keys. Failures return `SignatureDoesNotMatch` / `InvalidClientTokenId` /
  `MissingAuthenticationToken`. `AssumeRoleWithWebIdentity`/SAML, presigned
  URLs, and S3 public reads stay exempt. Enforcement also exposed and fixed a
  synthetic-behavior bug: the Amplify handler had minted a fake presigned S3 URL
  with no signature (BUG-2636).
- **Google Cloud** consolidated its access-token minters onto one process-stable
  RS256 key, published a JWKS, and added a data-plane middleware verifying the
  bearer's signature, issuer, audience, and expiry (`UNAUTHENTICATED`
  otherwise). **Azure** reused its RS256 verifier, added the missing audience
  check, and wired it as an ARM data-plane middleware (`invalid_token`
  otherwise). Both exempt their token minters, discovery/JWKS, metadata/IMDS,
  health, and OCI registries.

Because the simulators now enforce, every consumer that had relied on them not
checking was made faithful: the SDK/CLI/Terraform suites fetch a real token or
sign with the seed; the sockerless backends dropped their `WithoutAuthentication`
(GCP) and `fakeCredential` (Azure) fakes for a real GCE metadata token source
and `DefaultAzureCredential` — differing from the real cloud only in
coordinates; the relying-party suite signs its AWS IAM provisioning and bearer-
authenticates its GCP and Azure provisioning; and the console browser e2e, which
reaches the enforcing simulator without an identity provider, moved its
reads-real-data assertions to the authenticated relying-party path.

Separately, the AWS ECS Terraform harness left subprocesses running past its
deadline (BUG-2569, P1): the ECS *service* model launched real Docker containers
to satisfy `DesiredCount`, and no-command images (`alpine`/`busybox`) crash-
looped forever so the service never reached steady state and containers leaked.
The service is now a control-plane state machine that reaches
`runningCount==desiredCount` with a COMPLETED deployment synchronously (real
workloads run through `RunTask`, which nothing changed), and the `internal/tfsim`
harness gained the process-group + deadline-watchdog reaper the main harness
already had. `TestStackProductionShape` converges and terminates with zero
leaked containers or processes.

## 2026-07-23 — Completed the Microsoft Azure portal on both fidelity axes

The Azure portal got the same treatment the Google Cloud and AWS consoles did,
completing the set: every simulator console now reads only real cloud APIs, and
no `/sim/v1/*` dashboard endpoint remains on any of them.

Data: the portal reads the real Azure Resource Manager APIs — Azure Container
Apps jobs, Azure Functions sites, Azure Container Registry, and Azure Storage
accounts — enumerating subscriptions and listing each provider across them, plus
Azure Monitor's Log Analytics query API (a distinct host and token audience from
Azure Resource Manager, reached by listing Log Analytics workspaces and running a
Kusto query against each). The invented `/sim/v1/*` dashboard endpoint is
deleted. The operator's Shauth assertion is exchanged through **Microsoft Entra
Workload Identity Federation** — the `client_credentials` grant with a JWT-bearer
`client_assertion` — against a registered federated identity credential; the
simulator now verifies the assertion against the identity's credential issuer,
subject, audience, and RS256 signature (via `go-oidc` + `go-jose`) and issues an
Azure token, where it previously issued one to any client_credentials request.
This is the same client code and identifiers a real client uses against Azure,
differing only in the endpoint, tenant, and identity coordinates, with no
sim-aware branch. The relying-party suite proves the whole path with a live
Shauth operator token: an administrator registers a managed identity and a
federated identity credential trusting the operator's own issuer, subject, and
audience, and Microsoft Entra exchanges the operator's assertion for an Azure
Resource Manager token.

Visual: the portal was already built to the Azure portal's layout — the blue
header, the "Microsoft Azure" wordmark, the command bar, the Essentials strip,
and the grouped service menu. It got a **Fluent-style inline-SVG icon set**
(approximating Fluent UI System Icons, MIT, drawn in-repo and self-contained) on
the command bar, the status pills, the service-menu and Essentials chevrons, the
header search, and the theme control, replacing the placeholder Unicode glyphs.
The browser suite pins the header blue and the command, status, and search icons
structurally so the Azure look cannot regress unseen.

## 2026-07-23 — Completed the Amazon Web Services console on both fidelity axes

The AWS console got the same treatment the Google Cloud console did, in one
branch covering both axes: real cloud data and the real visual language.

Data: the console reads the real AWS APIs — Amazon ECS, AWS Lambda, Amazon
ECR, Amazon S3, and Amazon CloudWatch Logs — over the console's own
server-side Shauth federation, replacing an invented `/sim/v1/*` dashboard
endpoint that was deleted. The operator's Shauth id_token is exchanged through
`AssumeRoleWithWebIdentity` against a registered IAM OpenID Connect provider;
the simulator now verifies the web identity token against that provider's
issuer, audience, and RS256 signature and reports the token's real subject
(it previously returned a hardcoded one). Each read is signed in the browser
with Signature Version 4 over the returned session credentials using Web
Crypto — the same client code and identifiers a real client uses against AWS,
differing only in the endpoint and federation coordinates, with no
sim-aware branch. ECS tasks are enumerated the way the real console shows
them: list clusters, then per-cluster list and describe. The relying-party
suite exercises the whole path with a live Shauth identity and a role carrying
a read policy, so an unpermitted role reads as the real 403 rather than silent
empties.

Visual: the console was rebuilt to the Cloudscape Design System, graded
side-by-side against the live reference with tokens read from its computed
styles rather than from memory. Open Sans — Cloudscape's own console font,
since Amazon Ember is proprietary and is approximated, said plainly — is
vendored as a woff2 subset; Cloudscape's action blue and rounded containers
are applied from the design system's own values; the dark navigation header
carries a global search field and a tool cluster (notifications, settings,
support, theme); and a Cloudscape-style inline-SVG icon set (SIL OFL / drawn
in-repo, self-contained) sits on the status pills and the table's
search-prefixed filter and refresh control. The browser suite pins the font,
the action blue, the container radius, and the header and table controls
structurally so the AWS look cannot regress unseen.

## 2026-07-23 — Rebuilt the Google Cloud console to the console's real visual language

The console recognisably evoked Google Cloud but sat far below what was
achievable — almost no icons, a fallback typeface, generic glyphs, and an
"identity unavailable" error rendered into its own chrome. A side-by-side
against the live console made the gap plain, and it turned out I had been
grading against memory with structural-only tests and an overclaimed "presents
as". This closed most of the gap, working from the console's own computed styles.

Design tokens are the values the live console paints — its blue-tinted page
background, the left-anchored active pill and its colour, the primary blue.
Roboto is vendored as a latin woff2 subset for body text (the display face,
Google Sans, is not redistributable and is approximated, said plainly); icons
are Material Symbols Outlined vendored as inline SVG paths, so the console is
self-contained — a real icon on every navigation item, the header tool cluster,
the filter, sort and column controls. Both are Apache-2.0. The account is an
avatar opening a menu with the identity and sign-out, neutral rather than an
error when unauthenticated; the empty state is completed. The browser suite now
pins the visual work structurally so it cannot regress to a sketch unseen. The
information architecture stays a deliberate divergence — one rail for the
resources the simulator implements rather than the real per-product navigation.

## 2026-07-23 — Read the last Google Cloud resources from their real APIs

Cloud Run jobs already read the real Cloud Run API; the console's other four
resources still read sockerless-invented `/sim/v1/*` endpoints with a trimmed
shape. They now read their real cloud APIs through the same federated,
coordinate-only path — Cloud Run functions from Cloud Functions v2, Artifact
Registry from its repositories, Cloud Storage from the JSON API, and Cloud
Logging from `entries:list` — each with a detail page on the real resource and
each rendering the true shape.

The overview counts each resource from the same real list its page reads,
rather than a summary endpoint, and reports whether those APIs answered rather
than a synthetic health signal. With the last consumer gone, the Google Cloud
dashboard — every `/sim/v1/*` route including the summary — is deleted; the
console reaches the cloud only through real APIs at configured coordinates. The
browser suite creates a bucket, a repository, and a log entry through the real
APIs and asserts the console lists each and opens its detail; the relying-party
suite is unchanged and still green.

## 2026-07-23 — Made the Google Cloud console reach the cloud only through real APIs and coordinates

The console had federated the operator through `/auth/cloud-token`, an endpoint
the simulator served and the real cloud does not — coupling the data plane to
the simulator, so the same console pointed at real Google Cloud would have
needed a simulator-versus-cloud branch. The console now reaches the cloud
exactly as it would reach the real thing, differing only in coordinates.

The console's Shauth authentication is the console's own layer, not the
simulator's. It stays server-side — session, front- and back-channel logout,
the marker contract — and exposes the operator's assertion to the browser at
`/auth/federation-subject`. The browser federates that assertion at the cloud's
real Security Token Service (`POST /v1/token`) and calls the real cloud APIs
with the result. The Security Token Service endpoint, the cloud API base, and
the workforce pool provider the console federates through are coordinates the
console reads from its configuration; empty means its own origin, where the
simulator serves them, and a real deployment points them at Google Cloud.

The simulator-served credential broker and its sim-side workforce-provider
auto-provisioning were deleted. The provider is provisioned the way an
administrator provisions it — through the real Identity and Access Management
API — by the relying-party harness standing in for the administrator, and its
resource name reaches the console as a coordinate.

The relying-party suite reads the assertion from the console's auth layer,
exchanges it at the real Security Token Service with a live Shauth identity, and
drives the signed-in console through a real Cloud Run API read over that
federation. Login, logout, and the marker contract are unchanged, so the same
suite proves no regression. The rule is written in AGENTS.md: a simulator
console UI differs from a real-cloud console only in coordinates.

## 2026-07-23 — Read the real Cloud Run API from the Google Cloud console, over Shauth federation

The simulator consoles looked like their clouds but read data through
sockerless-invented `/sim/v1/*` endpoints that returned a hand-trimmed shape.
The Google Cloud console's Cloud Run jobs view now reads the real Cloud Run
Admin API — the `/v2/.../jobs` list and the job resource behind a new detail
page — and renders the true resource: status from the job's terminal condition,
unique ID, launch stage, timestamps, labels, and executions. The invented
endpoint was deleted.

A real console reaches those APIs with a credential federated from the signed-in
session, so the simulator gained the pieces it was missing. The Security Token
Service token exchange (`/v1/token`) performs Workforce Identity Federation the
way `sts.googleapis.com` does: it resolves the workforce pool provider the
audience names, verifies the subject token against that provider's OpenID
Connect issuer with real discovery, key set, signature, issuer, audience, and
expiry checks, and issues a short-lived federated access token. A console
credential broker on the operator-session boundary reads the operator's Shauth
assertion — already captured in the ui-auth session — and exchanges it for that
token, which the browser presents as a bearer credential. The raw assertion
never leaves the server, exactly as a real console keeps it.

Whether a credential is attached is a real deployment condition rather than a
fallback: a simulator wired to a single sign-on provider federates the operator
and every call carries a token, surfacing a broker failure; a simulator with no
identity provider runs unauthenticated, the mode the account control already
reports.

The token exchange is driven end to end by the official external-account
credential in the SDK tests, with the refusals that matter. The browser suite
seeds a job through the real Cloud Run API and asserts the console lists it and
opens its detail, proving live resources render. The relying-party suite drives
the whole federation with a live Shauth identity, brokering the operator's
assertion into a cloud token and checking a bearer token returns. The credential
issuance advanced BUG-2625; the remaining Google Cloud resources and the AWS and
Azure slices follow the same pattern (BUG-2635).

## 2026-07-23 — Gave the Google Cloud simulator the Google Cloud console's interface

The last of the three simulator interfaces now presents as its own cloud's
console, so an operator who knows any of AWS, Azure, or Google Cloud recognises
the simulator for that cloud.

The reference was the real Cloud Run Jobs page, captured from the console
itself: a light global header with a project chip and a wide central search, a
product navigation whose active item is a filled pill, inline text actions
beside the page title rather than a button group, a refresh pinned at the
right, a description sentence beneath the title, and a filter chip above a
table whose headers carry inline help. Empty states pair a dashed-cloud
illustration with a headline, an explanation, and the side effect of the
primary action, matching what the console shows when a resource has none.

Tables keep their column headers while loading, empty, or failed, so what the
resource is described by stays readable when there are no rows to infer it
from. Cloud Logging omits severity when it is the default, and the console
reads that as DEFAULT rather than a blank cell.

Both themes are carried, with the control in the top right. Contrast was
measured against the surfaces the browser actually paints rather than assumed
from the palette, and a test holds every enabled text role at or above the
4.5:1 WCAG AA requires — disabled controls excluded, since the requirement
exempts them and the console greys them deliberately. The link blue was moved
one step darker so a text action clears AA on white; it had measured 4.27:1.
The test names the role and the ratio when a colour regresses.

The header sizes to its contents and establishes a stacking context, and a
test asserts that every control it holds lies inside it — the property whose
absence let a click reach the breadcrumbs on the AWS console. Status is matched
on whole words, since a substring test reports success for failure states. The
table is built in the console's idiom rather than from a generic table library,
which the package no longer depends on.

## 2026-07-22 — Gave the Azure simulator the Azure portal's interface

The second of the three simulator interfaces now presents as its own cloud's
console. Microsoft publishes an annotated diagram of the portal shell, and the
simulator follows it: the blue global header with a wide central search, a
breadcrumb, a resource title carrying the resource type and the directory it
belongs to, a horizontal command bar of icon actions with unavailable commands
greyed rather than hidden, and a service menu with its own search and
collapsible groups. A search narrows the menu to what matched, opening a group
the operator had collapsed rather than hiding the match inside it.

Every resource pane leads with Essentials, the portal's two-column key/value
grid, before its table. Resource tables carry per-column sorting, selection,
filtering, and pagination, and keep their column headers while loading, empty,
or failed, so what the resource is described by stays readable when there are
no rows to infer it from.

Essentials states only what the query returned. An earlier draft asserted a
constant "Available" beside every resource, which would have reported health
the simulator had never been asked for.

Both themes are carried, with the control in the top right. Contrast was
measured against the surfaces the browser actually paints rather than assumed
from the palette, and a test holds every text role at or above the 4.5:1 that
WCAG AA requires — the tightest is 4.53:1, white on the portal's own header
blue, which leaves little room to drift. That test fails, naming the role and
the ratio, when a colour regresses.

The header sizes to its contents and establishes a stacking context, and a
test asserts that every control it holds lies inside it. On the AWS console a
fixed-height header left the sign-out control drawn outside the header box
where the bar below covered it, and clicks aimed at it reached the breadcrumbs
instead. The same shape of bug is now checked for here rather than waited for.

## 2026-07-22 — Reclaimed the microVM workspaces that filled the runner volume

The `sim (aws sdk)` job began with 89 GB free after its own cleanup and still
exhausted the runner volume, killing the runner process as it wrote its own
diagnostic log. Nothing identified the writer, because uploading the job log
needs the disk that was full.

The step now writes test output to a file under a watched budget and reports
the largest consumers on the volume when a threshold is crossed — while there
is still disk to report them on. That named the consumer immediately:
`/tmp/sockerless-firecracker` held 24.7 GB of the 26 GB a passing run consumed.

Each Firecracker machine staged a full copy of the root filesystem tree in
order to build an ext4 image from it, then kept both for as long as the machine
ran. The staging tree is now removed once the image exists, since nothing reads
it afterwards.

A machine that is killed rather than stopped never ran its own cleanup, so its
workspace — a root filesystem image each — stayed for the life of the host. A
machine now records which process its workspace belongs to, and a later machine
reclaims workspaces whose owner is gone. Workspaces too young to have recorded
an owner are left alone, so a machine on its way to starting is not swept out
from under itself.

## 2026-07-22 — Gave the AWS simulator the AWS console's interface

The three simulator interfaces were one generic application wearing three
accent colours: identical layout, navigation, typography, and components, with
only the tint and the navigation labels differing. An operator who knew any of
the real consoles recognised nothing.

The AWS simulator now presents as the AWS Management Console. It carries the
dark global header, a breadcrumb trail, a service navigation that groups
services the way the console groups them, `Resources (count)` page headers
beside their actions, and resource tables with per-column sorting, selection,
filtering, pagination, and the console's own empty states naming the resource
and the Region. The service overview states Region and service health before
counts, and each count links through to the resource that owns it.

Values the previous interface dropped are shown: log retention, stored bytes,
creation timestamps, and function state.

Both themes are carried, with the control in the top right of the header where
the console keeps it. Every text and surface pair was measured against the
rendered result rather than assumed: the lowest ratio is 4.97:1 in light and
5.25:1 in dark, against the 4.5:1 that WCAG AA requires for body text.

Status is matched on whole words. A substring test reported success for
failure states, because "unavailable" contains "available" and "inactive"
contains "active", and a green tick on a failed resource stops an operator
looking further. Callers that know the meaning pass it rather than having it
inferred from wording.

## 2026-07-21 — Qualified the real product user interface, not a protocol page

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards exposed the authenticated operator through
`data-shauth-user="<exact username>"` on the visible account control and
`data-shauth-sign-out` on the real sign-out control. The browser qualification
matrix asserted the visible username against the identity endpoint and signed
out by clicking the control a person clicks, so a deployment whose product
shell renders no user or no sign-out control failed qualification even when its
protocol endpoints answered correctly. The markers carried no authorization
meaning and replaced no accessible name or semantic element.

A stale required-status-check list on `main` still demanded the pre-shard
`sim (aws cli edge)` and `sim (aws cli ec2)` contexts, which the four-shard
matrix could no longer emit; the list was corrected to the shard contexts and
the drift was recorded as BUG-2633.

## 2026-07-21 — Completed the exact Shauth and bounded-release contracts

Sockerless Admin's logout-completion bridge remained public after local session
revocation and redirected only to Shauth's issuer-correlated completion
endpoint. Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards passed the exact Shauth `0fda680cba964e5768ed75a9c3e5b7230c418ca6`
contract against real PostgreSQL, Ory Hydra, freshly compiled relying parties,
and Chromium. Eight serialized application-and-direction flows proved direct
and catalog entry, relying-party and provider logout, exact completion
bridging, application-local signed-out return, reload, reauthentication,
global revocation, immutable release identity, anonymous fail-closed behavior,
active event-stream readiness, and validator-credential isolation.

The production build created every frontend bundle before compiling all 11
UI-bearing Go binaries, and a repository gate rejected ordering regressions
that could silently produce headless release artifacts. Every ordinary GitHub
Actions job declared an enforced timeout of at most 15 minutes. Historical
runtime evidence split the over-budget AWS edge and Amazon EC2 command-line
interface groups into four non-overlapping shards while preserving exact
single coverage of all 630 AWS CLI tests.

The nightly fuzz harness ran targets in bounded parallel batches with one Go
fuzz worker per target, retained truthful logs and crasher handling, and failed
on missing modules instead of skipping them. A complete one-second pass
exercised every discovered target in the AWS, Google Cloud, and Microsoft Azure
simulators and shared modules, core, Docker backend, and agent. The complete
test, lint, clean production-build, pre-commit, real authentication, and fuzz
gates passed together.

The required pre-push freshness gate also advanced every tracked Google Cloud
Storage consumer to v1.64.0. The complete Google Cloud Run, Google Cloud Run
Functions, shared Google Cloud backend, and standalone Google Cloud simulator
SDK suites passed with the reconciled module graphs.

## 2026-07-20 — Made simulator registry pushes portable and faithful

The Google Cloud Build and Azure Container Registry Tasks official SDK
harnesses shared one container-engine registry-policy utility. Docker Engine
continued to trust HTTP loopback registries natively, while Podman received an
exact scoped registry policy and reloaded that policy before the real build and
ordinary Docker-compatible push. Cleanup removed only the test-owned policy and
reloaded Podman again. The complete Google Cloud and Microsoft Azure official
SDK suites passed on macOS Podman, including real registry manifests and image
cleanup.

## 2026-07-20 — Scoped dependency freshness to repository source

The mandatory dependency freshness gate enumerated Git-tracked Go modules,
Terraform provider declarations, and GitHub Actions instead of walking
arbitrary nested directories in the worktree. User-owned untracked worktrees
therefore could not contaminate a repository release gate. The same pass moved
all three Google Cloud Secret Manager consumers and every workflow checkout
action to their current published releases, with the affected module graphs and
checks reconciled.

## 2026-07-20 — Made signed-out Shauth re-entry explicit

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
terminal pages exposed accessible, keyboard-visible `Sign in with Shauth`
controls instead of generic return actions. Admin linked to `/auth/shauth`; each
simulator linked to `/auth/oidc/login`. The standalone responses retained
no-cache headers, responsive layouts, semantic status text, and automatic
light/dark rendering.

The real PostgreSQL, patched Ory Hydra, Shauth, compiled four-relying-party,
and Chromium matrix logged out from every application, proved cross-application
session invalidation, exact app-local landing, and reload persistence, then
validated and clicked each exact Shauth control. Focused Go and Playwright
coverage locked the same labels and coordinates into each owning component.

## 2026-07-20 — Enforced Sockerless Admin administrator authorization

Sockerless Admin required the Shauth `admin` role at the one middleware boundary
shared by its operator user interface and APIs. An authenticated developer
received a no-cache accessible `403` page with a logout control, while API
requests received a JSON `403` before an operator handler ran. Administrator
sessions retained the complete operator surface.

Focused coverage drove the real topology manager and filesystem, proving a
developer could not persist a project while an administrator could. The full
PostgreSQL, patched Ory Hydra, Shauth, compiled relying-party, and Chromium
matrix also created a developer through Shauth's own administration interface,
authenticated both roles through the real OpenID Connect flow, proved the
developer denial, and persisted and removed an administrator-owned topology
project. The harness ran Admin from an isolated temporary working directory so
its real persistence proof never changed the repository topology.

## 2026-07-20 — Enforced release-aware GitHub Container Registry retention

The main-only operator and simulator publication workflow retained the newest
20 complete immutable releases for each of `sockerless-admin`,
`sockerless-simulator-aws`, `sockerless-simulator-gcp`, and
`sockerless-simulator-azure`. Its release-aware selector kept each 12-character
source tag together with its `-amd64` and `-arm64` images and deleted obsolete,
untagged, or otherwise unrecognized package versions. The publication gate
locked the native runners, direct OCI architecture manifests, two-platform OCI
index, immutable tag grammar, complete package matrix, and retention invocation
into pull-request continuous integration and pre-commit validation.

## 2026-07-20 — Made the Shauth relying-party matrix hermetic

The Sockerless Admin and AWS, Google Cloud, and Microsoft Azure simulator
single-sign-on harness built each production frontend before compiling its Go
server. Clean continuous-integration runners therefore exercised the same
embedded interfaces as local runs instead of silently falling back to headless
binaries and returning `404` for `/ui/`. The matrix used the exact Shauth
verified-email revision and passed the real PostgreSQL, Ory Hydra, and Chromium
direct-entry, catalog-entry, shared-sign-on, identity, app-local landing, and
global-logout contract.

## 2026-07-20 - Real Shauth Relying-Party Contract

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards passed one real browser contract against PostgreSQL, Ory Hydra, and
Shauth. The matrix entered every application directly and through Shauth's app
catalog, signed in once, verified each application identity, initiated logout
from every relying party, observed global cross-application revocation, landed
exactly on the initiating application's public signed-out page, reloaded that
page without restarting authentication, and proved protected re-entry failed
closed. The test pinned the exact CI-green Shauth revision that served all
browser assets locally.

Admin registered its OIDC Front-Channel Logout route outside the local-session
boundary, preventing provider logout iframes from being redirected into an
interactive login page after the initiating session had already been revoked.
Admin and the shared simulator authentication module supported
`client_secret_post`, revoked local state before provider discovery failures,
required the OIDC Back-Channel Logout event claim to be the exact empty object,
and accepted explicit HTTP development coordinates only on loopback hosts.
Both front- and back-channel logout remained correlated to trusted issuer,
session, subject, and replay identifiers.

The dependency freshness gate also advanced `actions/setup-node` to its current
major release for the new browser job. The generated README status badges were
refreshed by the repository's sanctioned pre-push badge hook.

## 2026-07-19 - Polished Simulator Consoles and Global Admin Logout (`fix/simulator-console-ui`)

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards now shared a polished responsive shell with saturated accessible
light/dark palettes, consistent navigation, service-specific resource names,
keyboard focus treatment, and a screen-reader skip link. The current compiled
Go servers passed real Chromium coverage across every dashboard and Admin's
live overview, component status, metrics, reload, containers, and operational
pages through the real Docker passthrough backend. Every bundle served the same
self-contained Sockerless browser mark and Admin no longer depended on an
external font host. The Admin harness removed
its synthetic HTTP backend, used collision-free ports, detected dead child
processes, and cleaned its process tree deterministically instead of silently
testing against another local service.

Sockerless Admin also became a complete Shauth logout participant. Its browser
sessions were tracked server-side, verified OIDC Back-Channel Logout tokens
revoked matching sessions by `sid` or `sub`, replayed `jti` values were
rejected, and the user logout control initiated RP-Initiated Logout through
Shauth's discovered `end_session_endpoint`.

The AWS, Google Cloud, and Microsoft Azure dashboards used the same first-party
OpenID Connect relying-party module rather than an infrastructure-specific
authentication proxy. Direct UI entry used authorization code + PKCE with
state and nonce validation; signed local sessions exposed identity and a POST
logout control; RP-Initiated Logout carried the ID-token hint; and signed OIDC
Back-Channel Logout revoked sessions by `sid` or `sub` with `jti` replay
rejection. Only UI, identity, and logout routes were protected. Every native
cloud API slice retained its existing authentication and wire behavior.

Every cloud and GitLab smoke image now copied the shared OpenID Connect module
required by the standalone simulator graphs. The Google Cloud Run and Azure
Container Apps GitLab images also carried the shared agent module required by
their backend graphs, and the legacy AWS GitLab image selected the intentional
headless simulator build. All affected images compiled successfully; the exact
Amazon Elastic Container Service continuous-integration image also passed all
15 real simulator/backend Docker lifecycle assertions. A pre-commit and
continuous-integration contract now rejects any smoke Dockerfile that loses a
required shared module or compiles a GitLab simulator with its browser bundle
absent.

## 2026-07-19 - Authenticated Simulator Dashboards and Truthful Release Validation

The AWS, Google Cloud, and Microsoft Azure simulator dashboards now use their
shared first-party OpenID Connect session coordinates for signed-in identity
and application logout. Their shared shell displays the authenticated operator
with accessible user details and a real logout control while leaving every
cloud API route unchanged. Image publication runs only after a push to `main` and emits
the immutable short-SHA manifest plus its explicit `-arm64` and `-amd64`
images.

The nightly fuzz harness now selects the intentional headless build, skips
nested Go modules until their own matrix entry, reports build or target failures
without calling them crashers, bounds per-target workers, and collects only new
minimized inputs. Root-context simulator image builds also exclude local
dependency and generated output directories. The previous workflow failures
were resolved at their shared build, resource, and artifact boundaries rather
than being recorded as nonexistent parser crashes.

The repository-wide core test also reconciled the Go workspace checksum set
with the current transitive module graph, so the required gate leaves a clean
worktree on subsequent runs.

## 2026-07-19 - Amazon ECS A-Record Service-Registry Fidelity (`fix/ecs-a-record-service-registry-validation`)

The AWS simulator now validates Amazon Elastic Container Service service-registry port coordinates against the registered AWS Cloud Map DNS record type. A-record services reject `containerPort` or `port` with the same invalid-parameter contract as Amazon ECS, while portless registrations preserve task ENI discovery. Focused official AWS SDK coverage creates the real Cloud Map A-record registry, proves the rejected port-bearing request, and proves the valid portless request.

## 2026-07-18 - Cloud-Independent API-Only Simulator Runtime Contract (`feat/api-only-runtime-capability`)

The Amazon Web Services, Google Cloud, and Microsoft Azure simulators now exposed a common `/health` capability document with the configured runtime and a `workloadExecution` flag. `SIM_RUNTIME=process` remained a generic API-only simulator coordinate rather than a deployment-platform mode: storage, queues, eventing, audit, and control-plane slices continued to use their real API implementations, while callers could reliably discover that container workloads were unavailable.

The Microsoft Azure Container Instances slice no longer fabricated a successful running group in API-only mode. It returned a documented deployment failure before persisting a workload that it could not execute, matching the simulator's explicit runtime capability rather than presenting synthetic state. Focused shared-server health tests and a no-user-interface Azure Container Instances process-runtime test validated the contract.

The AWS RDS official SDK test module now used the current client release required by the repository freshness gate, keeping the simulator's required CI dependency graph current.

## 2026-07-18 - Operator Console Liveness (`fix/admin-health-liveness`)

Sockerless Admin served `GET /healthz` as a small unauthenticated liveness endpoint. Amazon Elastic Container Service and Shauth managed-app monitoring could therefore distinguish a live operator console from protected user-interface routes without attempting to authenticate a health probe. The browser console, administration API, Shauth authorization-code flow, and logout routes remained protected by the existing Shauth middleware. Focused operator-console tests verified the exact successful liveness response.

## 2026-07-17 - Immutable Sockerless Operator and Simulator Images (`feat/publish-sockerless-admin-image`)

Sockerless now published fully baked Amazon Elastic Container Service-ready images for the Shauth-capable operator console and all three cloud simulators. The Amazon Web Services, Google Cloud, and Microsoft Azure simulator images embedded their existing production web interfaces rather than compiling with `noui`; each continued to serve the same protocol-faithful cloud API and UI from its real binary. The operator image embedded the production Admin interface and ran as an unprivileged user.

The release workflow built every image natively on ARM64 and AMD64 runners, published `:<short-sha>-arm64` and `:<short-sha>-amd64`, and composed only `:<short-sha>` as the multi-architecture manifest. It emitted no mutable branch, semantic-version, or `latest` tags. Local ARM64 release-image builds passed for all four images. The three simulator images started in their documented API-only coordinate and served both `/health` and `/ui/`; the Admin image completed its Go tests, production web build, and startup check.

The required dependency-freshness gate also found an Amazon S3 service-client release and a Google API release across seven independently resolved backend and dispatcher module graphs. All graphs now use their current releases with reconciled transitive dependencies. The complete freshness gate and the affected Amazon Elastic Container Service, AWS Lambda, Google Cloud Run, Google Cloud Run Functions, common-library, and runner-dispatcher suites passed.

The same gate then found the matching Amazon S3, Smithy, and Google API drift in the AWS and Google Cloud simulator SDK graphs. Those official-client graphs were refreshed, and both complete SDK suites passed against their real simulator servers.

The simulator lint bootstrap now retries transient golangci-lint download transport errors with explicit connection and total-time limits, while `pipefail` preserves a real installer failure. This prevented a transient TLS reset from being reported as a source-lint defect.

## 2026-07-16 - Shauth Operator Sign-In and Simulator Quality Gates (`feat/shauth-operator-console`)

The Sockerless operator console gained optional Shauth OpenID Connect authorization-code sign-in with discovery, PKCE, nonce, state, signed HttpOnly sessions, audience validation, role enforcement, identity display, accessible avatar semantics, and logout. It guarded only the browser console and its administration API; the AWS, Google Cloud, and Azure simulator endpoints retained their native cloud protocols without browser-auth middleware.

The simulator dead-code gate now preserved analyzer diagnostics instead of exiting silently. The reported Azure failure identified and reconciled the simulator's standalone Go module graph after the SQLite shared-module refresh. The AWS, Google Cloud, and Azure dead-code scans and Azure no-UI module suite passed after that reconciliation.

## 2026-07-15 - Standalone Bleephub and Bleeplab Extraction (`chore/extract-bleep-products`)

Bleephub and Bleeplab moved into the independent `e6qu/bleephub` and `e6qu/bleeplab` repositories without retaining Sockerless commit history. Each repository retained exactly one root commit authored by the e6qu noreply identity. Bleephub now owns its Go server layout, web application, SSH gateway, dqlite node, Terraform module, tests, and official GitHub Actions runner consumer harness. Bleeplab now owns its server, user interface, tests, and official GitLab Runner consumer harness.

Sockerless removed the product implementations, user-interface packages, Terraform module, product workflows, administration wiring, stale local paths, and obsolete build artifacts. Documentation now treats both products as external consumers. The Bleephub runner harness builds its own product image and the real Sockerless simulator/backend binaries from a named build context; its spawned-runner image uses that same loaded harness image. The Bleeplab runner harness follows the same real consumer model. Terragrunt configuration in `e6qu/infra` pins the standalone Bleephub Terraform module root commit.

## 2026-07-14 - Targeted Main Validation and Standalone Cloud Run Builds (`fix-main-ci-trigger`)

Azure Key Vault purge now modeled the documented accepted long-running-operation form. The simulator deleted the recoverable vault, returned `202 Accepted` with an absolute Location operation URI, and served a terminal zero-length `200 OK` at that URI. The terminal poll URI is explicitly allowed by the Swagger conformance ratchet because the documented Location target has no upstream Swagger path. This let the current generated AzureRM client complete `VaultsPurgeDeletedThenPoll` without attempting to poll an already removed deleted-vault resource. Focused Azure Key Vault SDK coverage and a real Dockerized AzureRM apply, idempotency plan, and destroy run passed.

The Azure Container Registry simulator now returned `properties.roleAssignmentMode` on registry reads and preserved an explicit requested setting. When the request omitted it, the simulator returned Azure's `LegacyRegistryPermissions` default, matching Microsoft.ContainerRegistry and preventing current AzureRM Terraform from proposing a perpetual in-place registry update. The command-line interface contract asserted that default. The macOS nested Azure Terraform harness also loaded its Buildx-built test image into Docker's image store before it started the inner test. A complete Dockerized AzureRM apply, idempotency plan, and destroy run passed.

The fully baked Bleephub release image retried each Ubuntu dependency installation transaction from a freshly downloaded package index. This made the native ARM64 build resilient to an Ubuntu archive publication race where a just-replaced package returned `404` after the prior index had selected it. A complete local ARM64 release-image build passed through both dqlite installation stages and final image export.

CI kept the complete validation matrix on pull requests and moved post-merge `main` work into a dedicated Bleephub publication workflow. Every merge built native AMD64 and ARM64 images on their matching runners, published them as `ghcr.io/e6qu/sockerless-bleephub:<short-sha>-amd64` and `:<short-sha>-arm64`, and composed `:<short-sha>` as their multi-architecture manifest. It published no mutable `main` or `latest` tag, retained only the newest 20 short-SHA releases and their architecture variants, and did not restart simulator, browser, build, Terraform, or runner checks after merge.

Each native architecture tag now published a direct OCI image manifest without a Buildx provenance index. This made its referenced architecture manifest anonymously retrievable from GitHub Container Registry, which Amazon ECS on AWS Fargate required before it could pull the ARM64 or AMD64 member of the public multi-architecture release.

Closed BUG-2591 by upgrading stale Amazon Cloud Map, AWS Lambda, and Amazon Simple Systems Manager Go service clients in the Amazon Elastic Container Service backend, AWS Lambda backend, Bleephub wake function, and AWS simulator software development kit module. The affected backend, wake, and simulator software development kit suites passed against the updated clients, and repository dependency freshness passed.

Closed BUG-2592 by making Bleephub site administrators authoritative for repository authorization. An external GitHub administrator with a registered SSH key could now read, push, and administer organization-owned repositories through the same Git Smart HTTP and SSH checks as every other Git client; focused SSH transport coverage and the complete Bleephub suite passed.

Retention resolved the repository owner's GitHub account type before calling the GitHub Packages API, so the user-owned `e6qu` namespace used `/users/` while organization namespaces used `/orgs/`.

The Bleephub idle controller now switched the public route back to wake and set the application plus every dqlite voter to zero in one Amazon Elastic Container Service control-plane pass. Amazon Elastic Container Service completed the real connection drain asynchronously, so the Lambda did not time out and leave a partial quorum running.

The Bleephub Terraform module now published a cache-controlled, non-sensitive startup document from a dedicated Amazon Simple Storage Service bucket through an explicit Amazon API Gateway route. The wake Lambda retained only capacity control and token-protected administrator status JSON, while the document visibly tracked startup, loaded the healthy Bleephub document without a browser refresh, and showed administrator Amazon Elastic Container Service counts plus direct Amazon CloudWatch Logs, Amazon CloudWatch idle-alarm, and Amazon ECS console links only after administrator-token authentication. The release workflow built the versioned startup ZIP as a GitHub Container Registry package and retained its newest 20 releases alongside the multi-architecture Bleephub images. Every authenticated and sign-in Bleephub page showed the immutable image version and publication timestamp embedded at release build time.

The wake module also used the current Amazon Lambda SDK release required by the dependency-freshness gate.

The Google Cloud dependency refresh also reconciled the Cloud Run and Cloud Run Functions module graphs under `GOWORK=off`. The release-matrix no-UI binaries now build with their standalone module metadata instead of requiring a workspace-mediated dependency selection.

## 2026-07-14 - Bleephub Terraform Module Relocation (`feat/bleephub-terraform-module`)

The reusable Bleephub Amazon Elastic Container Service on AWS Fargate module moved from the generic Terraform module tree to `bleephub/terraform`, together with its Amazon Web Services simulator apply/destroy coverage and pre-built wake-listener source. The wake build script and relocated test resolved repository paths from the new module location. The superseded checked-in Terraform root was removed so the private `e6qu/infra` Terragrunt repository became the single production environment owner. The module README documented its required inputs, hosted origins, output contract, and secret-safe GitHub OAuth configuration.

## 2026-07-13 - Bleephub Amazon Elastic Container Service on AWS Fargate Deployment (`feat/bleephub-hosted-compute-network-onboarding`)

Bleephub deployed in a dedicated eu-west-1 Amazon Elastic Container Service on AWS Fargate stack rather than the separate EDD infrastructure. The reusable Terraform module provisioned private application networking with fck-nat, an Amazon Simple Storage Service gateway endpoint, encrypted Amazon Simple Storage Service git/object buckets, Amazon Elastic File System-backed native dqlite voters, an internal Network Load Balancer, Amazon API Gateway public wake routing, an administrator origin, and a hardened SSH Git gateway. The fully baked ARM64 release image performed no build work at task start.

GitHub OAuth, administrator-provisioned local users, the e6qu-org administrator/developer mapping, Git Smart HTTP, and SSH public-key Git were wired through production configuration. Live verification created a repository, registered an ephemeral SSH key, pushed and cloned over SSH, cloned over HTTPS, used the official GitHub command-line interface against the live server, and confirmed the healthy UI/API routes.

The idle controller armed a five-minute API-request alarm after traffic, safely quiesced the Amazon CloudWatch alarm before shutting down, scaled application and dqlite services to zero, and restored the full quorum on a subsequent cold wake. A live cold wake restored all three dqlite voters and the application before returning successful health responses. The git bucket had versioning suspended and a noncurrent-version lifecycle rule to prevent retained historical object costs.

The production browser harness now started the real SSH Git listener with a disposable host key and advertised a port-aware `ssh://` clone coordinate whenever the configured SSH host included a non-default port. This preserved GitHub-style SCP coordinates for production port 22 while giving Playwright a valid local transport. The empty-repository page therefore rendered its real SSH selector under test. The embedded user interface also served a saturated Bleephub SVG favicon instead of returning the single-page application shell for the browser icon request. Focused real Chrome verification created a repository through the public API and confirmed both the SSH selector and favicon link; the complete Bleephub Go suite passed in 221 seconds.

The native ARM64 core continuous-integration job now applied an explicit eight-minute deadline to the complete Bleephub test package. The prior shared five-minute deadline expired while the final webhook timeout test was running after every preceding Bleephub test had passed; the package retained its complete coverage and the other core packages retained their five-minute deadlines.

The repository dependency-freshness gate found stale cloud and supporting Go module pins before the timeout fix could be pushed. The affected Bleephub, cloud-backend, simulator, runner-dispatcher, agent, and command modules were updated to the current versions required by that gate. Dependency freshness then passed and the complete Bleephub suite passed in 218 seconds with the updated graph.

The primary continuous-integration workflow now ran for pull requests targeting `main` and for every push to `main`. A merged change therefore received the same independent post-merge validation as its pull request rather than leaving the protected branch without a run.

The required freshness gate also surfaced newly published Google Cloud and supporting module releases before the CI repair could be pushed. The affected modules, including the Google Cloud common backend's Cloud Build and Cloud Run clients, were upgraded and validated by the same gate.

## 2026-07-12 - GitHub Marketplace Publisher and Buyer Product (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2548 through BUG-2560 and removed GitHub Marketplace from BUG-2523. GitHub App and OAuth App owners created durable draft/published listings, dedicated signed webhooks, delivery history, and free, flat-rate, or per-unit monthly/annual plans through authenticated settings. Publisher REST plan and account reads required the owning GitHub App's JSON Web Token or OAuth App's Basic client credentials, kept unrelated publishers isolated, preserved GitHub's production and `stubbed` shapes, returned empty collections rather than null, and excluded confidential webhook configuration from public buyer listings.

Authenticated buyers browsed a GitHub-organized Marketplace, searched saturated app cards, compared plans, selected a personal or administered organization account, started trials, completed a GitHub App installation or OAuth App installation-URL handoff, and managed upgrades, downgrades, and cancellations. Upgrades began immediately; paid downgrades and cancellations waited for the billing boundary; free/trial cancellations began immediately; and purchased, changed, cancelled, and ping events used the listing-owned webhook. Subscription identity included listing, account type, and account ID, preserving multiple app purchases and colliding User/Organization numeric identifiers.

Marketplace listings, plans, webhook configuration/deliveries, subscriptions, pending changes, and installations survived SQLite reload. New subscription plus GitHub App installation creation committed in one SQLite transaction before either webhook began, and real closed-storage coverage proved that failure left no memory or installation residue. Plan/listing edits and deletion enforced active-purchase and published-plan invariants. The obsolete `/internal/marketplace/purchases` route and synthetic global free plan were removed.

The routed buyer directory/detail and GitHub App publisher editor retained GitHub's hierarchy while using a candy-saturated purple, blue, cyan, pink, green, and gold palette in both themes. Real Chromium also exposed and fixed the GitHub App dialog's opaque manual-redirect mistake: App Manifest creation now followed the real same-origin redirect and converted the code from its final URL. Expected absent publisher listings used a nullable `200` browser adapter instead of console-error `404` probes.

The complete Bleephub Go suite passed in 216 seconds; the user-interface suite passed 48 files / 334 tests; TypeScript, production build, and the unused-export gate passed with the tracked current-`knip` deprecation only; the complete real-Chromium suite passed 31/31; the complete official `go-github` suite passed; and the Dockerized official `gh` command-line interface harness passed 136/136. The complete all-files pre-commit gate also passed. Visual inspection confirmed distinct, legible light and dark Marketplace surfaces and the saturated discovery treatment. Cleanup removed 22 GiB of disposable Go build cache, temporary hook/package caches, 21 stale Amazon Elastic Container Service simulator task containers, and unused images without touching active services or volumes, increasing local free space from 31 GiB to 54 GiB.

## 2026-07-12 - GitHub CodeQL Producer and Code Security (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2535 through BUG-2540 and removed CodeQL database production from BUG-2523. Bleephub accepted the official GitHub CodeQL Action uploads-host request with a raw ZIP body, language, name, and real commit object ID; validated safe finalized CodeQL database or legacy database bundles with a language dataset; persisted archives in the object byte store; and removed the arbitrary internal base64 seed route. Public list, get, download, and delete behavior used GitHub's `contents` read/write permissions, honored repository-selected GitHub App installation tokens, protected private database and variant-analysis bytes, and returned GitHub-compatible download metadata.

Database replacement used immutable content-addressed object keys and preserved the prior metadata and bytes across object-store, SQLite, or cleanup failure. SARIF ingestion required a fully qualified ref and real repository commit, accepted GitHub Actions installation credentials with `security_events` permissions, preserved UTF-8 payloads, and created durable analyses even for valid zero-finding runs. The official producer and browser therefore shared truthful git coordinates instead of fabricated branches or all-zero object IDs.

The repository Security page was reorganized around GitHub-style Code scanning navigation, finding filters and detail, CodeQL database management, analyses, and SARIF upload. Its light and dark themes retained GitHub surface hierarchy while using saturated blue, cyan, purple, pink, and gold treatments. The account token hero also moved onto valid shared background, status, and elevation tokens, closing BUG-2541 and the prior CI gradient failure.

Closed BUG-2542 through BUG-2547 while making the browser and hygiene proof strict: the Code Security scenario used unambiguous real-commit locators, selected dark mode through the user menu, waited for the accepted producer response and rendered analysis, the user-interface API module no longer exported an unused single-alert helper, and SARIF ingestion preserved every run in multi-language or multi-configuration documents. The complete real-Chromium suite passed 30/30, the user-interface suite passed 46 files / 330 tests, TypeScript and `knip` passed, the complete Bleephub Go suite passed in 204 seconds, the official `go-github` suite passed, and the Dockerized official `gh` command-line interface harness passed 130/130.

## 2026-07-12 - Fine-Grained Personal Access Tokens (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2531 and BUG-2532 and removed fine-grained personal access tokens from BUG-2523. Authenticated account settings created durable `github_pat_` credentials for a user or active organization membership, constrained them to one resource owner, all/selected/no repositories, explicit repository and organization permissions, and an optional expiration, and displayed the secret exactly once. The polished GitHub-organized account page listed active, pending, revoked, and expired credentials, exposed organization-owner approval decisions, deleted owned credentials, and retained saturated, legible light/dark presentation.

Runtime authentication distinguished classic and fine-grained credentials. Pending, expired, revoked, cross-owner, unselected-repository, and ungranted-permission access was denied; repository inventories omitted inaccessible private resources while retaining GitHub's public-resource behavior; deletion removed authentication and associated request/grant state; API insights retained the fine-grained token identity; and SQLite reload preserved the complete credential contract. Organization request and grant REST administration became GitHub App-only with the official `organization_personal_access_token_requests` and `organization_personal_access_tokens` permission names for targeted installation and user access tokens.

Closed BUG-2534 by extending the repository secret-scanning and push-protection detector to generated `github_pat_` credentials and Bleephub's generated classic credential length. Committing a live fine-grained token now creates the same GitHub personal access token alert class instead of bypassing detection.

Official `go-github` coverage created the credential through the browser producer, minted a real GitHub App installation token, listed and approved the request, and listed the resulting grant. The Dockerized official `gh` command-line interface harness created and authenticated a one-time credential and passed as part of the branch's 130/130 cases. Account component tests, the passing real-Chromium scenario, the complete Bleephub Go suite, 46 user-interface test files / 330 tests, typecheck, and the production build covered the implementation.

Closed incidental BUG-2533 after the required browser check exposed stale routed release-edit state. Saving an edit now reconciled the detail query, exited editor state, and kept uploaded assets available for download and deletion; focused component coverage preserved that transition.

## 2026-07-12 - Retained GitHub Classroom Product (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2527 through BUG-2530 and removed GitHub Classroom from BUG-2523. The six official read-only GitHub Classroom REST endpoints became organization-admin scoped and were exercised through current `go-github` types and the official `gh` command-line interface. The obsolete Classroom operator seed routes were removed.

Bleephub retained the browser product with saturated GitHub-adapted light/dark organization. Organization administrators created, renamed, archived, and deleted classrooms; managed linked or identifier-only rosters; created individual and group assignments with deadlines, repository visibility, student permissions, team limits, feedback pull requests, and command-based autograding; and exported or imported lossless transition bundles after repository migration. Invite URLs routed into the product and authentication preserved the requested destination.

Acceptance copied the real starter git tree into an organization-owned repository, granted each student access, serialized concurrent decisions, enforced group capacity without partial roster claims, created the configured Feedback branch and pull request, installed a real GitHub Actions workflow, and recorded the baseline commit. Classroom counters and grade exports derived subsequent commits, deadline submission state, completed job results, and available/awarded points from real repository and Actions state instead of management input. Classroom metadata, rosters, autograding configuration, acceptances, and transition identity survived SQLite reload.

The completeness audit also closed BUG-2520 through BUG-2522 and BUG-2524 through BUG-2526. Bleephub's shared light/dark visual system retained GitHub/Primer surface and semantic hierarchy while adding saturated blue, cyan, purple, pink, gold, and green brand/state treatments. Repository context chrome became full-width and organized around GitHub's primary tabs, content shortcuts, administrative overflow, real Watch/Star toggles, and an owner-selecting Fork workflow backed by the public REST API. An authenticated `/ui-data` viewer-state read prevented expected public existence-check `404` responses from becoming browser resource errors while mutations stayed public. Browser and repository-social tests became independently provisioned and route-aware.

The parity specification was reconciled against the implementation. It removed already-fixed GitHub App selection, installation webhook, and App-hook gaps; documented the REST/state/event/UI proof boundary; and identified the remaining GraphQL-schema, REST-semantic, page-level UI, and external-ingress work. It identified GitHub Marketplace and hosted-compute network settings as the two remaining operator-ingress domains; the Marketplace section above recorded the completed public replacement, leaving hosted-compute onboarding open.

The release-provider compatibility pass also closed BUG-2518 and BUG-2519 after CI exercised the new workflows. The official GitHub software development kit release lifecycle established a real initial commit and `refs/heads/main` through GitHub's Git Database API before creating a release. The routed browser release scenario reused the exact uploaded asset buffer when asserting its displayed size, so it continued through authenticated download and deletion without a divergent hardcoded byte count.

## 2026-07-12 - Bleephub Release Provider Completeness (`feat/bleephub-ui-api-completeness-audit`)

This branch continued from merged #791 and audited Bleephub's UI routes against its implemented public GitHub API and real state. It identified the release provider as a complete class gap rather than a single missing screen.

Closed BUG-2512 by replacing the transient read-only release list with routed repository release workflows. `/ui/repos/{owner}/{repo}/releases`, `/releases/new`, and `/releases/{id}` now support deep links and browser history; create, edit, draft/pre-release state, delete, object-backed asset upload, authenticated asset download, and asset deletion all use the public GitHub Releases API. The Code view links into the routed manager instead of trapping release state in a local tab.

Closed BUG-2513 and BUG-2514 by making release identity repository-scoped and git-backed. Updates verify ownership before validation or mutation. Creation and tag-name changes resolve an existing real tag or resolve `target_commitish` and create a real lightweight tag, while duplicate releases and unresolved targets return validation errors without changing release or git state.

Closed BUG-2515 by deriving release webhook and GitHub Actions activity from real lifecycle transitions. Complete release payloads now carry `created`, `edited`, `published`, `unpublished`, `prereleased`, `released`, or `deleted`, with GitHub's draft workflow semantics. Closed incidental BUG-2516 by removing every remaining asynchronous workflow-discovery call from pull-request REST/GraphQL and repository-dispatch handlers, eliminating mutable go-git read/write races across the eventing class.

Closed incidental BUG-2517 by upgrading Bleephub's Markdown parser from `github.com/yuin/goldmark` 1.8.3 to current 1.8.4 after the required pre-push dependency-freshness gate detected the drift.

Validation in this branch included:

```bash
bun run --cwd ui/packages/bleephub typecheck
bun run --cwd ui/packages/bleephub test
bun run --cwd ui/packages/bleephub build
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(Releases_|WebhookReleaseLifecycleActions)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(Releases_|WebhookReleaseLifecycleActions)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
```

## 2026-07-12 - Bleephub GitHub Pages Branch Publication (`feat/bleephub-pages-branch-builds`)

This branch continued from merged #790, which made GitHub Actions artifact deployments publish real GitHub Pages sites from object storage.

Closed BUG-2506 by replacing permanent shape-only Pages build queues with real branch publication. `POST /api/v3/repos/{owner}/{repo}/pages/builds` now resolves the configured legacy source branch and `/` or `/docs` subtree from git, treats `.nojekyll` as already-built static output, rejects symbolic links, submodules, unsafe paths, empty sources, and content over 10 GB, writes a deterministic TAR archive through the same transactional S3-compatible publication path as workflow deployments, serves the result, and persists the actual commit, duration, terminal build/site state, custom-404 state, digest, size, and deployment record. Object replacement writes and validates the new durable object before deleting the prior publication and rolls back the new object if replacement cleanup fails.

Closed BUG-2507 by shipping and executing the real GitHub Pages generation runtime. The release image now contains Ruby, Bundler, `github-pages` 232, Jekyll 3.10.0, and the complete GitHub-supported plugin/theme graph behind `bleephub-pages-jekyll`. Branch builds without `.nojekyll` materialize the real git source in an isolated workspace, invoke Jekyll in safe production mode with repository identity, bound captured build output to 1 MiB, archive only regular generated files, and publish through the same object transaction. Malformed sites persist real terminal Jekyll errors and create no deployment. Unconditional integration coverage built the actual release image and proved Markdown/Liquid generation plus object-backed serving against real git and the Amazon Simple Storage Service simulator.

Closed BUG-2508 by routing smart HTTP pushes, Contents API commits, and Git Database branch-reference creates, updates, and deletes through one committed-reference event path. Every branch mutation now records repository activity, emits the push webhook, triggers GitHub Actions workflows, synchronizes matching pull requests, and automatically builds a configured legacy Pages source branch. The event consumers run in a race-safe order against the shared git store, and coverage proved automatic publication through both Contents API and Git Database writes.

Closed BUG-2509 by serializing workflow-run `actor` and `triggering_actor` fields through the complete GitHub simple-user representation rather than an abbreviated webhook-only shape. Closed BUG-2510 by resolving git storage through canonical repository IDs and metadata coordinates through `full_name`, so committed-reference processing and Pages source reads do not dereference optional expanded owner representations.

Closed incidental BUG-2511 by upgrading Bleephub's Markdown parser from `github.com/yuin/goldmark` 1.8.2 to current 1.8.3 after the required pre-push dependency-freshness gate detected the drift.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(StaticPagesBranchArtifactValidation|PagesBuildsCRUD|PagesCreateUpdateShape)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(StaticPagesBranchArtifactValidation|PagesBuildsCRUD|PagesCreateUpdateShape)' -count=1
docker buildx build --load -f bleephub/Dockerfile.release -t sockerless-bleephub-pages-test .
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPagesJekyllBuildPublishesGeneratedSite' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Pages|pages' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(PagesBuildsCRUD|Dependabot_OrgAlerts|EnterpriseDependabotAlerts|SecretScanning_OrgAlerts|SecretScanning_PushProtectionBypasses|SecretScanning_PushProtectionBlocksGitDatabaseRefBeforeMutation)$' -count=1
```

## 2026-07-12 - Bleephub GitHub Pages Artifact Fidelity (`feat/bleephub-fidelity-sweep-next`)

This branch continued from merged #788, which hardened persisted repository ownership, Git provider and user-interface behavior, GitHub Apps authorization, GitHub Actions execution and runner contracts, container packages, and Projects v2 ownership.

Closed BUG-2502 by making GitHub Pages deployments consume real artifact bytes before reporting success. `POST /api/v3/repos/{owner}/{repo}/pages/deployments` now retrieves either the supplied artifact URL or the repository-owned GitHub Actions artifact, reads object-backed artifacts from S3-compatible object storage, rejects unreadable artifacts and metadata/byte-size mismatches without changing Pages state, and records the deployed byte count and SHA-256 digest. Coverage exercised both accepted inputs through Bleephub's real artifact-download data plane and object storage.

Closed BUG-2503 by completing the publication operation behind deployment success. Bleephub now validates the official GitHub Actions ZIP containing `artifact.tar` plus direct ZIP, TAR, and gzip-compressed TAR inputs; rejects links, path traversal, empty archives, and content over GitHub's absolute size limit; stores immutable published archives in S3-compatible object storage; advertises a usable Bleephub Pages URL; serves index files, clean URLs, static assets, HEAD responses, and custom `404.html`; gates private sites on repository access; reclaims superseded publication objects; and removes published bytes before Pages or repository deletion.

Closed BUG-2504 by validating GitHub Actions workflow identity before publication. The Pages deployment endpoint now verifies the Bleephub OpenID Connect token's RS256 signature, key identifier, issuer, audience, validity window, repository and repository identifier, environment, build SHA, and configured source branch. Malformed, altered, expired, cross-repository, cross-environment, wrong-ref, wrong-build, and wrong-audience tokens fail before artifact retrieval or state mutation.

Closed BUG-2505 by adding GitHub's distinct `pages` fine-grained repository permission to the authorization model. Pages writes require `pages: write`; private Pages reads require `pages: read`; classic `repo` scope continues to cover Pages; and repository `administration` permission no longer grants Pages access.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PagesDeployments_CreateStatusCancel|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PagesDeployments_CreateStatusCancel|PagesArtifactValidationRejectsUnsafeAndEmptyArchives|PagesPermissionIsDistinctFromAdministration|PagesHealthCheck|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
pre-commit run --all-files
```

## 2026-07-11 - Bleephub Public State Fidelity (`feat/bleephub-public-state-fidelity`)

This branch continued from merged #787, which moved CodeQL variant-analysis query-pack tarballs to object storage and made runner-log object-store failures preserve live process state.

Closed BUG-2491 by making persisted repository ownership strict. Repository reload now requires valid `owner_type` and `owner_id`, loads organizations before repositories so organization-owned repositories validate against real organization state, and fails loudly when persisted owner data is missing or inconsistent. Public repository listing and event paths no longer treat empty owner types as user repositories.

Closed BUG-2492 by removing the internal runner-submission image fallback. `/internal/exec/submit` and `/internal/exec/workflow` now require either an explicit `image` or `hostMode`, and tests that intend container execution pass `alpine:latest` explicitly instead of relying on hidden server-side defaulting.

Closed BUG-2493 by moving container-package coverage onto the real GitHub Container Registry-compatible data plane. Container package fixtures now publish blobs and manifests through the OCI/Docker Registry HTTP API v2 routes, package REST tests observe the resulting manifest/layer files, source coverage rejects new internal container-package seed calls, and `/internal/packages` rejects `container` package creation instead of leaving a parallel operator-only publish path.

Closed BUG-2494 by making Projects v2 GraphQL project creation resolve owners strictly. `createProjectV2` now requires the supplied `ownerId` to match a real user or organization GitHub node ID, returns a GraphQL error for unknown owner IDs, and does not mutate project state when owner resolution fails.

Closed BUG-2495 by removing the hidden execution-image default from public GitHub Actions workflow trigger and rerun paths. Push/event-triggered workflows, full-run reruns, failed-job reruns, and single-job reruns now preserve host-mode runner messages when the workflow YAML has no `container:` declaration, matching GitHub's runner contract instead of injecting `alpine:latest`.

Closed BUG-2496 by removing alternate base64 decoders from the GitHub Actions runner OAuth public-key path. Runner registration now accepts only the Azure DevOps/GitHub Actions runner protocol's standard base64 RSA modulus and exponent fields, and URL-safe or raw base64 variants fail loudly instead of creating a second public-key wire format.

Closed BUG-2497 by making GitHub Actions workflow parsing reject missing or invalid runner labels for normal jobs. Normal jobs now require `runs-on` to be a non-empty string or non-empty string list, reusable-workflow call jobs remain valid without runner labels, and job-list responses no longer invent `ubuntu-latest` when a directly seeded job lacks a definition.

Closed BUG-2498 by making public repository commit listing distinguish empty or broken git state from a successful empty history. `GET /api/v3/repos/{owner}/{repo}/commits` now returns GitHub's `409` empty-repository response when the default branch has no ref, returns a fail-loud service error when git storage cannot be opened or walked, and no longer reports `200 []` for a repository whose git history is unavailable.

Closed BUG-2499 by making Bleephub repository UI pages consume the new public commit-listing semantics faithfully. The UI now treats only GitHub's exact empty-repository `409` response as an empty commit history for display, while every other commit-listing conflict or storage failure still surfaces as an error.

Closed BUG-2500 / GitHub issue #789 by making organization repository creation honor GitHub App installation-token permissions. `POST /api/v3/orgs/{org}/repos` now authorizes installation tokens by target organization and `administration: write`, while installation tokens without that grant still receive `Resource not accessible by integration` and human tokens still require organization membership.

Closed BUG-2501 by separating Bleephub repository-page empty-history rendering from the public GitHub REST commit-listing contract. `GET /api/v3/repos/{owner}/{repo}/commits` still returns GitHub's `409` empty-repository response, while the authenticated `/ui-data/repos/{owner}/{repo}/commits` route maps only that exact empty git state to `200 []` for the browser UI. Storage and object failures still return service errors, and repository pages no longer emit strict browser-console resource errors for handled empty repositories.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PersistenceReload_(OwnerAndCountersAndState|OrganizationRepositoryOwnerIsValidated|RepositoryMissingOwner(Type|ID)FailsLoud)|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ConcurrencyGroups_(RepoAndRunEndpoints|CompletedRunReleasesLease)|SubmitWorkflow(RepoRefResolution|RejectsUnresolvedRepoRef)|Workflows_Dispatch|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPackages|TestContainerRegistry|TestLivePackages' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestProjectsV2GraphQL_(CreateProjectRequiresResolvedOwner|CreateProjectUsesResolvedUserAndOrganizationOwners|FieldValueKinds|ProjectLevelConnections)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(RerunKeepsRunIDAndBumpsAttempt|RerunFailedJobsCarriesSuccesses|RerunWorkflowJob_NewAttemptCarriesOtherJobs|Workflows_Dispatch|Workflows_DispatchUsesHostModeWhenWorkflowHasNoContainer)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(AgentRSAPublicKeyRequiresProtocolStandardBase64|OAuthToken|OAuthTokenRejectsMissingAssertion|OAuthTokenRejectsUnknownClient|RegistrationTokenRandom|GenerateJITConfig|RemoveToken)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ActionsPendingDeploymentReviewFlow|WorkflowParseRequiresValidRunsOnForNormalJobs|WorkflowParseReusableWorkflowJobDoesNotRequireRunsOn|WorkflowParse(ContainerAsString|ContainerAsObject|Env|JobOutputs|StrategyFailFast))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ListCommitsEmptyRepositoryFailsLoud|GetSingleCommit|CommitBranchesWhereHead|CommitPulls|CommitArchiveDownload)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ListCommitsEmptyRepositoryFailsLoud|UIListCommitsEmptyRepositoryReturnsEmptyHistory|GetSingleCommit|CommitBranchesWhereHead|CommitPulls|CommitArchiveDownload)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(InstallationTokenCreatesOrganizationRepositoryWithAdministrationPermission|InstallationTokenCreateOrganizationRepositoryRequiresAdministrationWrite|InstallationTokenDownscoping|CreateOrgRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
bun run --cwd ui/packages/bleephub test src/__tests__/api.test.ts
bun run --cwd ui/packages/bleephub test
bun run --cwd ui/packages/bleephub typecheck
cd ui && bunx turbo run build --filter="*bleephub*"
pre-commit run --all-files
```

## 2026-07-11 - Bleephub CodeQL Variant-Analysis Query Pack Objects (`feat/bleephub-codeql-variant-query-pack-objects`)

This branch continued from merged #786, which moved more Bleephub service bytes to object storage and hardened public GitHub-compatible ingestion, deletion, and official-client coverage.

Closed BUG-2489 by moving CodeQL variant-analysis query-pack tarballs out of SQLite metadata and into the configured object byte store. Variant-analysis rows now persist controller, actor, language, target, status, query-pack size, and object-key metadata; public query-pack downloads read the object store; persistent stores fail loudly without `BLEEPHUB_OBJECT_S3_BUCKET`; and controller-repository deletion purges query-pack objects before deleting repository metadata.

Closed BUG-2490 by making GitHub Actions runner-log upload and run-log deletion complete required object-store writes/deletes before mutating in-memory log, console, or timeline state. Fail-loud object-store errors now preserve the previously visible process state instead of leaving live state diverged from durable object storage.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(LogfilesUpload_(WritesObjectStore|ObjectStoreFailurePreservesState|AppendsBlocks|CapsAtFourMiBWithMarker)|JobLogs_ReadsUploadedLogFilesFromObjectStore|RunLogsDelete_ObjectStoreFailurePreservesState|ActionsRuns_DeleteLogs)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLVariantAnalyses_|PersistentServerStorageRequiresDurableGitAndObjectBytes|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata|TransferRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
bash -n scripts/bleephub-local-dev.sh
git diff --check
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Object-Backed Service Bytes (`feat/bleephub-object-backed-service-bytes`)

This branch continued from merged #785, which cleaned Codespace runtime/workspace state during repository deletion, hardened the AWS SDK simulator CI shard, and made persisted Bleephub require object-backed GitHub Actions artifacts, dependency caches, and runner logs.

Closed BUG-2471 by extending the object-backed byte-storage contract to release assets and GitHub Packages. Release asset uploads, package version files, and GitHub Container Registry blobs now write through the configured S3-compatible object store when it is present; SQLite stores metadata and object keys. Persisted startup and local development documentation now describe `BLEEPHUB_OBJECT_S3_BUCKET` as the required store for all durable service bytes, and release asset object-delete failures surface through the API and repository deletion path instead of being ignored.

Closed BUG-2472 by making public GitHub Packages file downloads read object-backed package file bytes. The metadata/listing path and the byte-serving path now use the same object storage source, so advertised package file REST URLs work for object-backed service bytes instead of looking only for local filesystem paths.

Closed BUG-2473 by making repository deletion fail loudly on required git-storage cleanup failures. Bleephub now purges filesystem or S3-backed git storage before deleting repository metadata; if S3 git-prefix cleanup cannot be resolved or completed, the delete returns an error and preserves the repository record and git storage index instead of logging and orphaning git objects.

Closed BUG-2486 by making repository deletion purge repository-owned GitHub Packages file bytes before deleting repository metadata. Object-backed and local package file bytes now go through the required cleanup path, so package byte cleanup failures surface as repository-delete errors instead of leaving durable package objects behind after package metadata is gone.

Closed BUG-2487 by moving GitHub CodeQL database archive bytes out of SQLite metadata and into the configured object byte store. CodeQL database rows now persist metadata, size, and object keys; public archive downloads read the object store; database deletion removes the object before deleting metadata; and repository deletion purges repository-owned CodeQL database archive objects before deleting repository metadata.

Closed BUG-2488 by moving artifact attestation Sigstore bundle bytes out of SQLite metadata and into the configured object byte store. Attestation rows now persist repository linkage, subject digests, predicate type, initiator, timestamps, and object keys; repository, organization, and user attestation list endpoints read bundle JSON from object storage; public attestation deletion removes object bytes before metadata; and repository deletion purges repository-owned attestation bundle objects before deleting repository metadata.

Closed BUG-2474 by upgrading stale AWS software development kit service modules found by the pre-push dependency freshness gate. The Amazon Elastic Container Service backend, AWS Lambda backend, and AWS simulator software development kit tests now use the latest published CloudWatch, Amazon Elastic Compute Cloud, and AWS Lambda service module versions required by the gate.

Closed BUG-2475 by moving the Bleephub go-github software development kit harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The SDK tests no longer create organizations through `/internal/orgs`, and source coverage rejects that operator-only route in the official-client harness.

Closed BUG-2476 by moving Bleephub public GitHub REST test organization setup onto GitHub Enterprise Server's public admin organization API. Public feature tests now use a shared `/api/v3/admin/organizations` helper for prerequisite organizations, while the only remaining direct `/internal/orgs` organization-creation calls are explicit operator-management coverage; a source guard rejects new direct public-test setup calls to the operator route.

Closed BUG-2477 by moving Bleephub public code scanning alert setup onto GitHub's public SARIF upload route. The shared code scanning alert helper now uploads SARIF to `/api/v3/repos/{owner}/{repo}/code-scanning/sarifs`, live-shape and campaign coverage use that public ingestion path, and SARIF rule severity/description metadata now flows into persisted alert state for filtering and downstream features.

Closed BUG-2478 by making the Bleephub UI typecheck pre-commit hook rebuild `@sockerless/ui-core` declarations before checking Bleephub. The hook clears stale ignored incremental build state, emits the required declarations, and then runs Bleephub `tsc`, so cleaning generated `dist` output no longer leaves the hook dependent on manual repair.

Closed BUG-2479 by making Bleephub secret scanning derive alerts from real repository content. Contents API writes now scan the new commit for supported provider secret patterns, Git Database branch reference creation/update scans commit targets, alert locations persist real commit/blob/path coordinates, and public secret scanning tests use committed secret patterns instead of an internal operator alert seed route.

Closed BUG-2480 by removing the undocumented `node_id` field from Git Database blob-create responses. `POST /api/v3/repos/{owner}/{repo}/git/blobs` now matches the OpenAPI response-shape ratchet.

Closed BUG-2481 by making Bleephub Dependabot alerts derive from public dependency graph snapshots and published security advisories. Repository security advisories now persist GitHub vulnerability package coordinates; successful default-branch dependency snapshots create matching Dependabot alerts from the global advisory database; publishing an advisory creates alerts from already submitted dependency snapshots; and the old operator-only Dependabot alert seed endpoint was removed.

Closed BUG-2482 by making the AWS simulator software development kit Amazon Elastic Container Service long-running task test poll real task state through `DescribeTasks`. `TestECS_TaskNoCommandStaysRunning` no longer assumes task startup completed after a fixed sleep, while still asserting that the no-command task reaches and remains `RUNNING` without container exit codes.

Closed BUG-2483 by making Bleephub secret scanning push protection mint bypass placeholders from protected public writes. Public contents writes and Git Database branch reference creation/update now detect enabled provider patterns before mutating git state, return a `422` push-protection response with a placeholder, honor active public bypasses for the matched token type, and no longer expose the internal operator placeholder seed route.

Closed BUG-2484 by removing the obsolete internal code scanning alert seed endpoint. Code scanning alert tests and downstream campaign/autofix coverage already created alert state through GitHub's public SARIF upload route, so `/internal/repos/{owner}/{repo}/code-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Closed BUG-2485 by removing the obsolete internal secret scanning alert seed endpoint. Secret scanning alert tests already created alert state from committed repository content and Git Database branch reference writes, so `/internal/repos/{owner}/{repo}/secret-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Validation in this branch included:

```bash
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistentServerStorageRequiresDurableGitAndObjectBytes|TestReleases_AssetBytesUseObjectStore|TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestReleases_AssetLifecycle|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestPackages_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'Test(DeleteRepoS3GitCleanupFailurePreservesRepo|GitDeleteCleanup|UnitDeleteRepo)$' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(DeleteRepoPurgesRepositoryPackageObjectBytes|PackageAndRegistryBytesUseObjectStore|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(Packages_|ContainerRegistry|PackageAndRegistryBytesUseObjectStore|DeleteRepoPurgesRepositoryPackageObjectBytes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLDatabases_(RoundTrip|BytesUseObjectStore)|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQL|CodeScanning|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(RepoAttestations_|OrgAttestations_|UserAttestations_|Attestations_CursorPagination|ArtifactMetadataAndAttestationPersistenceReload|PersistenceReload_DeleteRepoPurgesIssueAndPullChildren|PersistentServerStorageRequiresDurableGitAndObjectBytes)' -count=1
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
bash scripts/check-latest-deps.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitHub(CommandLineInterface|SoftwareDevelopmentKit)HarnessUsesAdminOrganizationAPI|TestAdminCreateOrg' -count=1
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'Test(Organizations|AppsInstallationTokenFlow|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooksSDK)$' -count=1)
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -count=1)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPublicFeatureTestsProvisionOrganizationsThroughAdminAPI|Test(GetOrg|UpdateOrg|DeleteOrg|ListAuthUserOrgs|CreateTeam|ListTeams|GetTeam|DeleteTeam|OrgMembership|RemoveMembership|TeamRepoPermission|ListUserTeams|GraphQLViewerOrgs|GraphQLOrganization|CreateOrgRepo|CreateOrgRepoExtended|ListOrgRepos|RepoOrganizationField|OpenAPIOrg|GetRepoInstallationHTTP|InviteFlow|PublicizeAndConcealMembership|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooks|Codespaces|AppsInstallationTokenFlow|CreateRepositoryInOrganization|Actions.*Org)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanning(AlertTestsUsePublicSARIFUpload|_ListAndFilter|_GetAndInstances|_PatchDismiss|_InvalidDismissedReason|_SARIFUploadCreatesAlerts|OrgAlerts|Autofix|AutofixEligibility)|LiveCodeScanning_FullFlow|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run ui-typecheck-bleephub --all-files
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestLiveSecretScanning_CRUD' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitData|TestUpdateRef|TestSecretScanning_GitDatabaseRefCreatesAlert|TestGetBlob|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestDependabot|TestLiveDependabot|TestEnterpriseDependabot|TestDependencyGraph|TestGlobalSecurityAdvisories|TestSecurityAdvisories' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
(cd simulators/aws/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run TestECS_TaskNoCommandStaysRunning .)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestRegisteredAPIv3RoutesExistInGitHubSpec|TestFuzzRoutePatternsMatchRegisteredRoutes|TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanningAlertTestsUsePublicSARIFUpload|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|CodeScanning|LiveCodeScanning|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(SecretScanningAlertTestsUseCommittedContent|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|SecretScanning|LiveSecretScanning|GitData|UpdateRef|CreateRef|CreateBlob|ListRefs|GetRef)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Repository Codespace Cleanup (`feat/bleephub-repository-codespace-cleanup`)

This branch continued from merged #784, which closed the broad Bleephub repository-deletion durable-state cascade for issue/pull request child state, repository-ID keyed rows, selected-repository references, deployment state, environment state, GitHub Pages deployment records, team grants, artifact metadata, source import records, dependency snapshots, SBOM exports, enterprise Dependabot repository-access IDs, labels, milestones, and reaction parent buckets.

Closed BUG-2468 by making repository deletion clean Codespace runtime state before deleting repository records. Repository deletion now removes backing Codespace containers and workspace directories through the same fail-loud path as direct Codespace deletion, and REST/GraphQL callers surface cleanup failures instead of deleting only the SQLite row.

Closed BUG-2469 by hardening the `sim (aws sdk)` CI job against GitHub-hosted runner disk exhaustion. The job now frees regenerable Go/Docker/apt caches before the large AWS SDK simulator shard, runs the prebuilt SDK test binary directly, and passes the prebuilt simulator binary into the SDK test harness instead of rebuilding the simulator during execution.

Closed BUG-2470 by making persisted Bleephub require object-backed GitHub Actions byte storage. Startup now refuses SQLite persistence unless the Actions artifact/cache/log byte store has been initialized from `BLEEPHUB_OBJECT_S3_BUCKET`, so a restarted service cannot advertise durable CI/CD records whose bytes lived only in memory or local files. The local development launcher fails loudly until object-store coordinates are supplied, and the persistence documentation now names the same requirement.

Closed BUG-2391 through BUG-2398 by wiring repository REST/GraphQL metadata to persisted repository, git, Pages, and viewer-access state. Licensed repositories exposed `Repository.licenseInfo`; discussion/issues/wiki settings and merge-method settings flowed through REST and GraphQL; Pages capability, pushed timestamps, archival timestamps, template provenance, and repository permissions stopped using constants or fabricated defaults.

Closed BUG-2399 by rebalancing the AWS Command Line Interface simulator appdata/appdata2 shards while preserving required check names.

Closed BUG-2400 and BUG-2401 by making pull request GraphQL status rollups use both REST commit statuses and check runs, and by adding GitHub's top-level `avatar_url` to REST commit status responses.

Closed BUG-2414 by making release asset upload follow GitHub's raw upload contract. Bleephub now registers the advertised `/api/uploads/repos/{owner}/{repo}/releases/{id}/assets{?name,label}` route, reads metadata from the query string, stores the raw request body with the request content type, and no longer accepts multipart/form fallback bytes.

Closed BUG-2402 through BUG-2405 and BUG-2413 by making Codespaces fail loudly. Codespace records are persisted only after workspace/container creation succeeds; image pull failures do not fall back to `ubuntu:latest`; start/stop/delete return errors on required backend failure; delete preserves state after failed container cleanup; and random-name generation requires cryptographic entropy rather than timestamp fallback.

Closed BUG-2406 by making OAuth device flow require browser approval. Device codes start pending, token polling returns `authorization_pending` until approval, and the final token belongs to the approving logged-in user.

Closed BUG-2407 by snapshotting code-quality setup records at the store boundary so failed validation cannot mutate persisted setup state through escaped slices or timestamps.

Closed BUG-2408 through BUG-2412 by removing fabricated Actions repository/ref/SHA context. Repository-scoped workflow paths resolve refs through real git storage and reject unresolved refs; repo-less internal submissions omit repository context instead of claiming `bleephub/test`; missing repository scope fails job/message construction loudly; webhook test deliveries require a real default-branch commit; and run-control tests seed real repositories before exercising repo-scoped runs.

Closed BUG-2415 by making the Bleephub runner UI use GitHub's repository-scoped Actions runners REST endpoint for its primary inventory. The page no longer fetched `/internal/sessions`, and its coverage asserted the public repository and runner routes while rejecting internal session access.

Closed BUG-2416 by replacing unexplained shorthand in Bleephub UI source comments with descriptive dashboard, user profile, and organization page names.

Closed BUG-2417 by making GitHub Pages deployment creation advertise the GitHub-compatible status URL and making status/cancel lookup resolve the public deployment/build identifier as well as the internal record ID.

Closed BUG-2418 by centralizing checked cryptographic randomness for Bleephub token, secret, invite-code, advisory, gist, and OpenID Connect token identifier generation. Ignored `crypto/rand.Read` calls were removed, timestamp fallback identifiers were removed, and a source guard now rejects unchecked entropy reads.

Closed BUG-2419 by making GitHub Actions artifact finalization and signed-download URL lookup use the workflow run backend identifier when it is supplied. Same-name artifacts from concurrent runs no longer cross-finalize or cross-download, matching the existing list scoping.

Closed BUG-2420 by scoping public GitHub Actions run, attempt, job, log, cancel, rerun, delete, artifact, concurrency, and protection endpoints to the repository named in the GitHub REST path. Global workflow run IDs and stable job IDs no longer resolve across repositories after only checking the requested repository's readability.

Closed BUG-2421 by making Bleephub notification thread identity type-safe. Issue and pull request notification threads now use distinct typed IDs, read/done/subscription state keys no longer collide across resource types, advertised notification thread URLs use `/api/v3/notifications/...`, and old numeric-only notification store helpers were removed.

Closed BUG-2422 through BUG-2425 by moving Bleephub account-management, audit-log, and OAuth UI paths off operator-only management routes. Organization and team management now uses GitHub Enterprise Server/public GitHub REST organization and team routes instead of `/internal/orgs` and `/internal/teams`. User administration now uses GitHub Enterprise Server user list/create/delete/site-admin routes instead of `/internal/users`; Bleephub also persists account suspension state and rejects suspended user tokens with `403`. The audit-log page now reads organization audit logs through `/api/v3/orgs/{org}/audit-log` using GitHub's phrase/order query model, and the server applies ascending audit-log order. The OAuth page now starts web/device flows and polls device tokens through `/login/oauth/authorize`, `/login/device/code`, and `/login/oauth/access_token` instead of rendering pending server-side codes from `/internal/oauth/state`.

Closed BUG-2426 by backing Bleephub browser sessions with real stored credentials. `/login` now requires a stored personal access token for the submitted account, rejects arbitrary password strings and mismatched tokens, refuses suspended accounts, and invalidates existing browser sessions when the account becomes suspended. OAuth web-flow consent and device-flow approval therefore run under a real authenticated Bleephub user instead of a login-name-only session.

Closed BUG-2427 by requiring real registered OAuth clients across Bleephub OAuth flows. Device-code issuance now rejects unknown `client_id` values, device-token polling requires the same client ID that issued the code, authorization-code consent requires a registered OAuth App or GitHub App client, and the token exchange validates the matching client secret before minting a user-to-server token.

Closed BUG-2428 by keeping the Bleephub OAuth UI on the same registered-client contract as the service. The OAuth flow controls no longer rely on a fake default client identifier, and the user-entered registered `client_id` is included in the web authorization URL, device-code request, and device-token polling request.

Closed BUG-2429 by fixing hook-discovered stale coverage and dead UI types. The pending-deployment review flow fixture now creates a real workflow file through the public contents API before submitting a repo-scoped workflow, the GitHub Enterprise Server-only user-administration and Pages deployment status routes are explicitly allowlisted in the route-spec guard, and obsolete runner-session TypeScript exports were removed after the runner UI moved to GitHub Actions public runner endpoints.

Closed BUG-2430 by making the local Bleephub Go pre-commit hook truthful during the temporary local Docker outage. During the outage, the local hook ran the non-Docker Bleephub suite while Docker-backed Codespaces lifecycle coverage remained fail-loud in CI instead of silently pretending the missing local Docker socket was covered.

Closed BUG-2431 by upgrading the stale AWS and Google Cloud Go modules surfaced by pre-push dependency freshness. The affected Amazon EC2 software development kit, Google API client, and Google Cloud Firestore module pins were brought to their latest published versions, and dependency freshness passed again.

Closed BUG-2432 by removing hidden admin-owned identity defaults from GitHub App seed configuration. Seeded GitHub Apps now require an explicit existing owner user, installations require an existing target user or organization with a matching target type, persisted app owners are validated on load, and app JSON no longer fabricates a Simple User when app owner state is corrupt.

Closed BUG-2433 by renaming the Bleephub runner integration harness's Google Cloud service-account credential generation from fake service-account JSON to simulator service-account JSON. The harness still generated a real RSA key and drove the Google client JWT signing and token exchange path, with only the token endpoint coordinate pointed at the simulator.

Closed BUG-2434 by restoring the local Bleephub Go pre-commit hook to the full Bleephub suite after Docker compatibility returned on the host. The temporary non-Docker skip script was removed, so Docker-backed Codespaces coverage ran locally again instead of being deferred to CI.

Closed BUG-2435 by making Docker-backed Make targets load local images correctly across Docker frontends. The shared build helper uses `docker buildx build --load` when Buildx is available and legacy `docker build` otherwise, so smoke, Bleephub runner, Bleeplab runner, and Bleephub `gh` command-line interface harness images are available to the following `docker run` step under Docker Engine and Podman compatibility.

Closed BUG-2436 by correcting the Bleephub `gh` command-line interface documentation to name the actual required `Bleephub GitHub command-line interface` CI job.

Closed BUG-2437 by making GitHub Actions workflow dispatch resolve GitHub `ref` inputs through git storage the way official clients send them. Dispatch now accepts full refs, branch names such as `main`, tag names, and raw commit SHAs, stores the resolved ref/SHA on the workflow run, and still returns a loud `422` for unresolved refs. The real `gh workflow run ci.yml --ref main` path passed in the Docker-backed command-line interface harness.

Closed BUG-2438 by removing the remaining user-facing Bleephub UI dependency on operator-only metrics/status/storage diagnostics. The overview and metrics pages now aggregate workflow runs, jobs, job conclusions, and online runners through public GitHub REST repository Actions routes; tests assert those pages do not call `/internal/metrics`, `/internal/status`, or `/internal/storage`. The storage-coordinate page was removed from the routed UI instead of wrapping non-GitHub persistence details in a user-facing product surface.

Closed BUG-2439 by deleting the dead `formatUptime` helper after process uptime stopped appearing in user-facing Bleephub pages.

Closed BUG-2440 by splitting the Bleephub production UI bundle at real route and dependency boundaries. `App.tsx` lazy-loads page modules through the router, and Vite now emits explicit vendor chunks for React, TanStack, YAML, cryptography, and miscellaneous third-party code without raising Vite's chunk warning threshold. The production build no longer emits large-chunk or circular-chunk warnings.

Closed BUG-2442 by updating Bleephub Playwright end-to-end coverage to the public GitHub Actions metrics contract. The Operations console now expects the `Workflow runs` metrics label exactly, the metrics page checks the `GitHub Actions throughput` heading, and fault-injection coverage fails `/api/v3/user/repos` instead of the removed `/internal/metrics` diagnostic route.

Closed BUG-2443 by making the AWS simulator's Amazon Simple Queue Service `ReceiveMessage` honor long polling. Empty receives now wait up to `WaitTimeSeconds`, available messages still return immediately, and invalid wait times outside the real 0-20 second range fail loudly. The AWS SDK test harness now runs the main simulator at warning level so successful request traffic cannot flood CI logs.

Closed BUG-2444 by adding the missing AWS Budgets CloudTrail event-source mapping. AWS Budgets management calls now record the real `budgets.amazonaws.com` event source instead of emitting fail-loud "no eventSource mapping" warnings, and the mapping unit coverage pins the service prefix.

Closed BUG-2445 by exposing GraphQL `Release.immutable` from the same persisted immutable-release state used by the REST endpoints. Repository release connections, release-by-tag lookup, and latest-release lookup now derive the field from repository-level toggles plus organization all/selected enforcement instead of hiding the field to make official clients fall back.

Closed BUG-2446 by making GraphQL pull request status-rollup connections expose the official GitHub command-line interface count-by-state fields from the same commit-status and check-run stores that back the node list. Actions-created check suites now persist their workflow-run identifiers, workflow name, and workflow file metadata, so `CheckRun.checkSuite.workflowRun.workflow.name` resolves from real Actions state instead of returning null.

Closed BUG-2448 by updating the GraphQL sweep test header to name GitHub command-line interface version 2.96 as the source for the replayed GraphQL shapes used by the current status-rollup coverage.

Closed BUG-2447 by persisting GitHub Actions workflow runs and archived attempts in SQLite. Run creation, dispatch state transitions, cancellation, deployment review, rerun archive/restore, startup-failure runs, repository rename/delete, and run deletion now keep the durable run records coherent; non-terminal runs reload as completed/cancelled because runner dispatch state is process-local and cannot truthfully continue after a service restart.

Closed BUG-2449 by returning fail-loud GitHub API errors for public secure-random generation paths that had still panicked. GitHub App manifest conversion, seeded GitHub App secrets, OAuth App creation, OAuth web/device token issuance, installation tokens, gist create/update/fork identifiers, security advisory and CVE identifiers, Classroom invite codes, OpenID Connect signing keys/token IDs, hosted-compute network settings/configuration IDs, GitHub Actions runner registration/removal tokens, and Actions cache download tokens now propagate entropy failures to their HTTP handlers; cache reservation avoids creating partial cache records when token generation fails.

Closed BUG-2450 by moving OAuth App token reset and scoped-token creation onto the error-returning user-to-server token path. Reset now mints the replacement before revoking the original token, so entropy or persistence failure returns a fail-loud GitHub API error without destroying the existing credential.

Closed BUG-2451 by moving the Docker-backed `gh` command-line interface parity harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The harness no longer calls `/internal/orgs`, and Go coverage now rejects that operator-only route in the official-client harness.

Closed BUG-2452 by making the Bleephub enterprise UI consume the configured enterprise slug at runtime. `/health` now reports the service's `BLEEPHUB_ENTERPRISE_SLUG`, Enterprise page copy displays that slug, and all enterprise UI REST helpers build `/api/v3/enterprises/{enterprise}/...` paths from that runtime coordinate instead of hardcoding the default `bleephub` slug.

Closed BUG-2453 by removing the Bleephub UI test setup's localStorage warning source. The setup now installs jsdom localStorage without first touching Node's warning-producing localStorage getter, so localStorage-backed auth paths still run and Vitest output stays clean.

Closed BUG-2454 by moving fine-grained personal access token generation onto an injectable full-read entropy helper. The helper now returns a normal error when secure randomness is unavailable, and entropy failure is covered directly alongside the other credential helpers.

Closed BUG-2455 by persisting Bleephub gist state in SQLite-backed service storage. Gists, comments, stars, forks, histories, comment counters, and sequence counters now reload as durable service state instead of disappearing on process restart.

Closed BUG-2456 by replacing the stale Bleephub persistence bucket inventory comment with a pointer to the actual `loadBucket` registrations. The code no longer carried a duplicate manual list that drifted when durable state buckets changed.

Closed BUG-2457 by making repository deletion purge the persisted child state attached to the deleted repository's issues and pull requests. Issue comments, issue events, sub-issue links, issue dependency links, pull request reviews, and pull request review comments no longer survived a SQLite reload or attached to a later repository that reused issue or pull request IDs.

Closed BUG-2458 by extending repository deletion to repository-ID keyed rows and selected-repository references. Artifact attestations, repository activity, clone traffic, watch subscriptions, GitHub App selected repositories, installation token repository scopes, organization Actions settings, runner groups, Actions secrets/variables, agent secrets/variables, Dependabot access and org secrets, Codespaces org secrets, Copilot coding-agent permissions, private registries, immutable-release enforcement, and code-security attachments no longer survived deletion with the old repository ID.

Closed BUG-2459 by extending repository deletion to deployments, deployment statuses, environments, environment branch policies, environment protection rules, and GitHub Pages deployment records. Those repository-ID keyed rows no longer survived SQLite reload or attached to a later repository that reused the old ID.

Closed BUG-2460 by making deployment deletion purge the deployment's status rows from memory and SQLite with the deployment record.

Closed BUG-2461 by moving team repository access lists and permission overrides during repository rename and transfer, and by removing those team grants when the repository is deleted.

Closed BUG-2462 by moving organization artifact storage/deployment metadata `github_repository` references during repository rename and transfer, and by deleting artifact metadata rows for a deleted repository.

Closed BUG-2463 by adding source imports, dependency graph snapshots, generated SBOM exports, and enterprise Dependabot repository-access IDs to the repository deletion cascade.

Closed BUG-2464 by adding Copilot coding agent tasks, issue field values, and CodeQL variant-analysis target rows to the repository deletion cascade. Deleted repositories and their issues no longer left those durable rows behind for a reloaded or recreated repository to inherit.

Closed BUG-2465 by adding Projects v2 content items to the repository deletion cascade and by making project deletion clear its in-memory content index. Deleted repository issues and pull requests no longer left project items behind after reload or ID reuse.

Closed BUG-2466 by adding notification state to repository deletion and rename cascades. Deleted issue and pull request threads no longer left read, done, or subscription state behind for later ID reuse, and repository rename/transfer moved repo-scoped notification read markers to the new full name.

BUG-2441 stayed open because the current Bleephub UI unused-export toolchain still emitted Node's `DEP0205 module.register()` warning after `knip` was upgraded from 6.15.0 to the current 6.23.0 release. The gate passed and dependency freshness showed no newer `knip` version.

Validation in this branch included focused Bleephub Go tests for repository metadata, pull request status rollups, commit statuses, release asset upload, Codespaces name/catalog behavior, OAuth device flow, code-quality setup, Actions secrets/variables, workflow dispatch/internal submission, repository webhook test delivery, and run-control fixtures. The latest combined focused command was:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ViewDefaultFields|TestPersistenceReload_CheckRunsAndSuites' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_WorkflowRunsAndAttempts|TestWorkflowRunsListNewestFirst|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestApproveWorkflowRun_ReleasesGatedRun' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked|TestCreateGist|TestGitHubApp|TestOAuth|TestSecurityAdvisories|TestClassroom|TestActionsOIDC' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth(App|Check|Reset|Revoke|Scope)|TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestGitHubCommandLineInterfaceHarnessUsesAdminOrganizationAPI|TestAdminCreateOrg|TestCreateOrg|TestListAuthUserOrgs' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestExistingRoutesUnaffected|TestGHApiRoot' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestEntropyHelpersReturnErrors|TestOrgPATGrantRequests' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_GistsCommentsStarsAndForks|Test(CreateGist|UpdateGist|DeleteGist|StarUnstarGist|ListStarredGists|ForkGist|GistComments|ListGistsForAuthUser|ListPublicGists|GistCommitsAndRevision)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestSubIssues_|TestIssueDependencies_BlockedBy|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteDeploymentPurgesStatuses|TestPersistenceReload_DeploymentsStatusesEnvironments|TestDeployments_Lifecycle' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestProjectV2' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata|TestNotifications_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

They passed with sandbox escalation for loopback listeners.

Docker compatibility was available again through Podman 6.0.1, container listing worked, and a minimal container run passed:

```bash
docker version
docker ps
docker run --rm alpine:3 true
```

The full Bleephub Go pre-commit test command passed after Docker compatibility returned:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The Bleephub UI validation passed after the public Actions metrics and route-level code-splitting changes:

```bash
bun run test src/__tests__/EnterprisePage.test.tsx src/__tests__/api.test.ts src/__tests__/OverviewPage.test.tsx
bun run test src/__tests__/OverviewPage.test.tsx src/__tests__/MetricsPage.test.tsx
bun run test
bun run typecheck
bun run build
npx knip
bun outdated knip
```

`npx knip` exited successfully but still emitted the Node `DEP0205 module.register()` warning tracked as BUG-2441. `bun run build` completed without Vite large-chunk or circular-chunk warnings.

The Docker-backed Bleephub `gh` command-line interface parity harness passed with the Docker-compatible Podman runtime:

```bash
make bleephub-gh-docker-test
```

It passed again after the OAuth App token-management entropy fix, after the official-client organization provisioning fix, and after the runtime enterprise-coordinate fix, each time with 117 checks passing and 0 failing.

It also passed after gist state became durable and after repository deletion began purging persisted issue and pull request child state, with 117 checks passing and 0 failing.

The focused Bleephub Playwright coverage for the public Actions metrics UI and error paths passed after rebuilding the embedded UI binary:

```bash
bun run test:e2e -- e2e/bleephub.spec.ts --grep "Operations console|Global navigation|Metrics page"
bun run test:e2e -- e2e/errorPaths.spec.ts
```

The focused AWS simulator validation for the Amazon SQS long-polling and CloudWatch-to-Amazon SQS path passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestSQS_ReceiveMessageHonorsLongPollingWaitTime|TestSQS_ReceiveMessageRejectsInvalidWaitTimeSeconds|TestCloudWatch_OKActionsDispatchedToSNS' .
```

The full AWS simulator software development kit target passed with the Docker-compatible Podman runtime:

```bash
make sdk-test SDK_TEST_TIMEOUT=600s
```

The AWS simulator CloudTrail event-source mapping unit test passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -run TestAWSEventSourceCoversAllServiceSlices .
```

The focused AWS simulator software development kit rerun for AWS Budgets, process-mode CloudWatch/SNS/SQS, process-mode Amazon Elastic Container Service managed Amazon Elastic Block Store, and Amazon SQS long polling passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestBudgetsCRUDSDK|TestECS_ManagedEBSRunTaskProcessMode|TestCloudWatch_AlarmSNSActionToSQS_ProcessMode|TestSQS_ReceiveMessageHonorsLongPollingWaitTime' .
```

The GraphQL release immutable-state validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestRepoGraphQL_ReleasesConnection|TestImmutableReleases_OrgSettingsAndRepoEnforcement|TestImmutableReleases_SelectedRepositories' -count=1
```

The full Bleephub Go package test also passed after the GraphQL release schema change:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

The workflow-dispatch `ref` input validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestWorkflows_Dispatch' -count=1
```

The Docker-backed `gh` command-line interface parity harness passed:

```bash
make bleephub-gh-docker-test
```

The dependency freshness hook also passed:

```bash
bash scripts/check-latest-deps.sh
```

The GitHub App seed validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestSeedPreRegisteredApp|TestSeedAppIdempotentAndBadKey|TestPersistence_RoundTripAppsInstallationsTokensRepos' -count=1
```

The runner harness shell syntax check also passed:

```bash
bash -n bleephub/test/run-integration.sh
```

The full local Bleephub Go hook command also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The runner UI validation also passed:

```bash
bun run test src/__tests__/RunnersPage.test.tsx
bun run typecheck
```

The GitHub Pages deployment validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPagesDeployments_CreateStatusCancel|TestPagesHealthCheck|TestPagesBuildsCRUD|TestPersistenceReload_PagesBuildIDSequence' -count=1
```

The checked-entropy validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestGitHubApp|TestOAuth|TestPagesDeployments_CreateStatusCancel|TestSecurityAdvisories|TestClassroom' -count=1
```

The Actions artifact run-scoping validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestArtifact(CreateUploadFinalize|FinalizeScopesByWorkflowRunBackendID|ListReturnsFinalized|Download)|TestGetSignedArtifactURL' -count=1
```

The Actions repository-scoped run/job validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsRunAndJobEndpointsScopeIDsToPathRepository|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestActionsJobs_(Get|Logs)|TestActionsArtifacts_ListRunArtifacts' -count=1
```

The notification thread identity validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestNotifications_(ListAndRead|ThreadIDsSeparateIssuesAndPullRequests|ThreadSubscription|RepoScoped|SinceAndBefore|ParticipatingFilter)|TestNotificationThreadMarkDone' -count=1
```

The user/organization/team UI route validation also passed:

```bash
bun run test src/__tests__/api.test.ts
bun run typecheck
```

The GitHub Enterprise Server user administration route changes also passed compile-only Go validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The focused runtime Go test for the user administration routes did not execute locally because the sandbox denied loopback binds and both escalated attempts timed out in the automatic approval reviewer before execution.

The audit-log public-route validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestAuditLogRecords' -count=1
```

The OAuth UI endpoint validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The browser-authentication validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The registered OAuth client validation used the same focused command and compile gate after the token endpoint required registered client IDs and web-flow client secrets:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The OAuth UI registered-client validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The hook-discovered fixture/spec/type cleanup also passed focused validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsPendingDeploymentReviewFlow|TestRegisteredAPIv3RoutesExistInGitHubSpec' -count=1
npx knip
```

## Recent Merged Context

- **#782 - Persist Bleephub repository metadata and permissions from real state.** Repository license/settings/Pages/pushed/archive/template/permissions fields moved to real persisted state; AWS Command Line Interface simulator shard balance was corrected without changing required contexts.
- **#781 - Bleephub GitHub Apps, Actions, storage, and repository fidelity.** Actions artifacts/caches/logs moved to object storage; S3 filesystem tests used the AWS simulator; GitHub Apps moved to Manifest/browser installation flows; public Actions runner/workflow paths replaced internal paths; metadata persistence became SQLite-only.
- **#779 - Bleephub pull request/release fidelity.** Pull requests, reviews, releases, action downloads, CodeQL fixtures, and repository rename/transfer/delete behavior derived from real git/object storage and public GitHub-compatible paths.
- **#778 - Open issue sweep and class hardening.** Fixed the actionable open issues except upstream-blocked AzureAD and tightened mutable store snapshots across simulators.
- **#774/#773 - Bleephub UI and stress hardening.** The UI became a functional GitHub clone, docs were swept, fuzz/load/concurrency coverage found races and scale bugs, and store/indexing hot paths were hardened.
- **#770/#750/#747 - Bleephub API/UI expansion.** Large REST/UI parity waves added many GitHub surfaces and pages; old operation-count detail lives in those PRs.

## Foundation Summary

- Docker-compatible cloud backends are stateless and map Docker concepts onto cloud primitives.
- AWS, GCP, and Azure simulators are real cloud API slices with conformance/coverage ratchets and official client coverage.
- Bleephub implements GitHub Enterprise Server-shaped REST, GraphQL, Actions, GitHub Apps/OAuth, repositories, issues, pull requests, releases, packages, webhooks, checks/statuses, Pages, and UI surfaces, with more fidelity work still active.
- GitHub Actions runner and GitLab docker-executor topologies are sim-proven across container-capable backends; live-cloud validation remains open under BUG-1075.

## Shauth operator-console authentication

Sockerless-admin gained optional Shauth OpenID Connect browser authentication.
When all production coordinates were configured, the console performed
authorization-code sign-in with PKCE and nonce validation, verified the ID
token, accepted developer and administrator roles, and used short-lived signed
HTTP-only sessions. Its shared application shell displayed the signed-in name,
role, initial avatar, and logout control. Local operator use stayed unchanged
when no Shauth coordinates were configured, while partial or insecure
production configuration failed at startup. The Amazon Web Services, Google
Cloud, and Microsoft Azure simulator API endpoints were not wrapped because
their real SDK, command-line interface, and Terraform contracts remained
unchanged.

## OpenID Connect logout protocol hardening

Sockerless Admin and the shared simulator user-interface authentication module
preserved the configured issuer exactly, rejected issuer and public coordinates
containing user information, constrained discovered logout endpoints to the
configured issuer origin, and required same-origin browser evidence for logout.
Back-channel logout accepted only bounded
`application/x-www-form-urlencoded` POST bodies, rejected query tokens,
validated `iat` and the required logout event as a JSON object, and consumed
each `jti` atomically with `sid`/`sub` session revocation. Admin retained a
validated ID token only for its owning session, bounded its session by the ID
token expiry, and used the client identifier when no ID-token hint remained.
Both Admin and the simulators returned an explicit public no-cache signed-out
page after the shared Shauth session ended, so logout did not immediately enter
a new sign-in flow.

## Current-source browser validation

The shared backend Playwright harness built the current web interface and Go
binary for every run instead of reusing an untracked executable, and launched
each server through its native command-line or environment coordinate. Cloud backend
suites started the corresponding real Sockerless simulator in API-only process
mode and provisioned their prerequisite Amazon ECS cluster, Google Cloud Storage
bucket, or Azure resources through the public cloud API surface. All seven
backend interfaces validated status, navigation, resources, metrics, and their
declared favicon in 77 browser scenarios. Their HTML stopped loading Google
Fonts at runtime, leaving each production bundle self-contained. Continuous
integration gained an explicit browser matrix for Admin, every simulator, and
every backend, while pre-commit and pre-push validation covered the shared
browser shell scripts. Each Playwright web server allowed bounded cold Go
dependency compilation in continuous integration before the harness applied
its separate 30-second runtime-health deadline, while individual browser tests
retained their 30-second timeout.

## Simulator dashboard authorization boundary

The AWS, Google Cloud, and Microsoft Azure simulator dashboards registered
their `/sim/v1/*` data handlers through the same first-party OpenID Connect
authorization boundary as the rendered operator interface. Unauthenticated
browsers could no longer read dashboard inventory behind a protected shell,
while health probes and native cloud API routes retained their existing
protocol-specific contracts.

## Direct architecture release manifests

The release workflow disabled provenance attestations on each native ARM64 and
AMD64 build so the explicit architecture tags resolved directly to OCI image
manifests instead of single-platform indexes with anonymous attestation
children. The manifest job verified both architecture media types and rejected
any generic short-SHA index whose platform set differed from exactly Linux
ARM64 and AMD64. This preserved the generic multi-architecture image for Amazon
Elastic Container Service and Kubernetes while keeping the explicit tags
usable by consumers that require a single-architecture image manifest.

## Expiring back-channel logout qualification

Sockerless Admin and the shared simulator identity module required every OIDC
Back-Channel Logout token to carry an expiry later than the validation time.
The real Shauth matrix registered all four relying-party back-channel paths,
kept the public browser coordinates on their loopback origins, and rewrote only
Ory Hydra's container-to-host delivery coordinates. The browser exercised
direct and catalog entry, shared sign-on, logout from every application,
application-local signed-out return, and fail-closed re-entry. Each compiled
relying party recorded successful signed back-channel acceptance, so the
matrix could not pass solely through front-channel iframes.

## Amazon ECS attached-container task generations

Reusing a stopped attached container created a fresh Amazon ECS execution
generation. The pending record reset to Docker's created state, every start
owned a new wait channel, and a delayed poller removed only its own channel.
While the new task was pending, cloud recovery no longer selected a historical
stopped task, so attach bound to the current task's CloudWatch stream instead
of replaying the previous cycle. Default Docker networking was normalized to
bridge semantics before task tagging and cloud-state reconstruction. A real
simulator/backend integration test ran two scripts through the same attached
container ID and received each cycle's distinct output.

## Independent Sockerless Admin session credentials

Sockerless Admin required a dedicated browser-session signing secret of at
least 32 bytes whenever Shauth OpenID Connect was enabled. The confidential
OpenID Connect client secret remained limited to client authentication, so
provider credential rotation no longer invalidated locally signed state or
session values. Focused validation proved that only rotation of the dedicated
session secret invalidated existing signatures, and the complete real
PostgreSQL, patched Ory Hydra, Shauth, compiled relying-party, and Chromium
matrix passed with the separated credentials.

## Release-aware Shauth relying-party validation

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards implemented Shauth's standard `/auth/validation` contract. Each
authenticated page exposed the verified username, email, role, and immutable
application release through exact machine-readable fields and used the
application's real global logout action. Anonymous requests returned an exact
`303` to the application's own signed-out page, while arbitrary bearer material
could not authenticate a relying party. The deployed authentication
configuration required a commit or container digest so Shauth could validate
the release actually serving each public origin.

The continuous-integration harness pinned and verified a clean Shauth source
revision before starting its real PostgreSQL and patched Ory Hydra stack. It
built the current production bundles and binaries for Admin and all three
simulators, confined passwordless validator credentials to Shauth, and rejected
their presence in relying-party process environments. Eight serialized browser
jobs covered catalog and direct entry for every application, exact identity and
release fields, relying-party global logout, application-local signed-out
return, reload persistence, reauthentication, and provider logout with witness
revocation. Sockerless Admin cached validated provider discovery metadata behind
a bounded initial lookup, preventing logout requests from hanging on repeated
discovery, and validation-page content security policy allowed only the exact
Shauth origin required by the real redirect chain.

The mandatory pre-push dependency audit also advanced the Amazon Web Services
Organizations SDK test client to its current patch release. The complete
official SDK module and the repository-wide dependency freshness gate passed
with the updated module graph.

## Containerized simulator outer-host propagation

A containerized AWS simulator resolved the outer runtime's existing
`host.docker.internal` or `host.containers.internal` IPv4 coordinate before
falling back to its own default route. It propagated that exact address to
nested workloads for metadata, callbacks, and user-supplied endpoint
coordinates, so Podman's simulator and workload networks no longer confused
the simulator gateway with the actual host.

The Bleeplab runner harness added a targeted real Amazon ECS workload check
that required the exact Bleeplab health response from inside the nested task.
The same run completed the full GitLab-style pipeline, compiled and consumed
an artifact, and reached Redis through the build pod's service alias.
Sockerless's Shauth harness also isolated the standalone validator's Go module,
so the same build succeeded when Shauth was checked out beneath Sockerless's
workspace in continuous integration. The browser job selected the Go toolchain
from that pinned Shauth module, preventing the provider's compiler requirement
from drifting behind Sockerless's own toolchain.
