# Sockerless deployment recipe

Committed, runnable infrastructure that hosts Sockerless Admin, the three
cloud simulators, and a Shauth identity stack (Shauth + Ory Hydra +
PostgreSQL) at persistent origins — every console registered as a Shauth
OpenID Connect client, federation resources provisioned through each cloud
simulator's real APIs, and a live-origin smoke test proving Shauth sign-in
and every data plane. This is PLAN.md § "Console Self-Service" phase 4.

The multi-architecture images `.github/workflows/publish-container-images.yml`
already publishes are the artifacts; this directory is the coordinates and
orchestration that turn them into a running deployment.

## Contents

| File | Purpose |
|---|---|
| `compose.yaml` | Registry-image deployment. Pulls `ghcr.io/e6qu/<image>`, gated on `SOCKERLESS_TAG`/`SHAUTH_TAG`. |
| `compose.build.yaml` | Source-build deployment. Builds every image from this repository's own Dockerfiles. Standalone — see "Why two files, not an override" below. |
| `.env.example` | The full environment schema. Copy to `.env` and fill in every value. |
| `Caddyfile` | The TLS-terminating reverse proxy every persistent origin is served through — see "Why every origin is HTTPS" below. |
| `hydra.yaml`, `shauth-postgres-init.sql` | Shauth's own required companion config (identical to what Shauth's own `compose.yaml` mounts). |
| `provision.sh` | Idempotent: registers OIDC clients, provisions federation resources against the real cloud simulator APIs, boots the Azure console profile. |
| `smoke.sh` | Live-origin smoke test: health, Shauth discovery, console redirects, each data plane's authenticated/unauthenticated contract, `sockerless login`. |

## Topology

```
                                    ┌───────────────────────────────┐
   browser  ─────────────────────▶ │             caddy              │  ← the ONLY published port;
   (real HTTPS to each hostname)   │  TLS termination by hostname    │    every origin below is
                                    │  (SNI), one shared port         │    reached exclusively through it
                                    └───┬──────┬──────┬──────┬──────┬─┘
                                        │      │      │      │      │
                            ┌───────────┘      │      │      │      └───────────┐
                            ▼                  ▼      ▼      ▼                  ▼
                        shauth          sockerless-admin  simulator-aws   simulator-gcp   simulator-azure-console
                     (identity, proxies    (Admin console)  (console+cloud  (console+cloud   (console ONLY —
                      hydra's public                         co-served,      co-served,       see below)
                      OIDC surface)                           one origin)     one origin)             │
                            │                                                                          │ server-side
                            │ backchannel logout (through caddy — see                                  │ federation
                            │ "Why every origin is HTTPS")                                             │ broker
                            ▼                                                                          ▼
                          hydra                                                            simulator-azure-cloud
                     (OIDC provider)                                                    (Azure Resource Manager +
                                                                                           Microsoft Graph + Log
                                                                                           Analytics data plane —
                                                                                           no browser reaches it
                                                                                           directly; reached over
                                                                                           the private compose
                                                                                           network for federation,
                                                                                           through caddy for reads)
```

Every simulator service is the same `simulator-<cloud>` image; what differs
between rows is configuration, never code (the project's "coordinates, not
branches" rule). AWS and GCP each run **one** simulator instance because
their console pages federate cross-origin directly against their own cloud
endpoints (AWS `AssumeRoleWithWebIdentity`, GCP STS token exchange both serve
real CORS). Azure runs **two** instances because real Microsoft Entra serves
no CORS for a `client_credentials` grant — the federation exchange has to
happen server-side, same-origin with the console (BUG-2640) — so
`simulator-azure-console` is the operator-facing portal that brokers the
exchange over the private compose network, and `simulator-azure-cloud` is the
plain Azure Resource Manager/Graph/Log Analytics data plane it federates
against and the browser then reads directly through caddy (real ARM/Graph
CORS, faithfully implemented by the simulator — never a co-served shortcut).

## Why every origin is HTTPS

Sockerless Admin and every simulator console validate their own OpenID
Connect issuer/public URL strictly at startup: HTTPS, or plain HTTP only for
the literal hostname `localhost` (see `cmd/sockerless-admin/shauth.go`'s
`validate()` and `the sockerless-cloud repository's ui-auth/auth.go`'s `isLoopbackHost`). Shauth
enforces the identical rule on every bootstrap app URL it reconciles,
*including* `backchannel_logout_uri`. A persistent custom hostname
(`shauth.localtest.me`, `admin.localtest.me`, …) reached over plain HTTP is
therefore rejected at boot by design on both sides of every OIDC
relationship — this is not a limitation this recipe works around; real
Microsoft/Google/AWS consoles have the same expectation of their own OIDC
relying-party configuration.

The `caddy` service is the fix and the only internet-facing surface: it
TLS-terminates every persistent origin by hostname (SNI) on one shared port
(`CADDY_PORT`, default `8443`) and reverse-proxies to the backend service
over the private compose network. `deploy/Caddyfile`'s `tls internal`
directive issues each hostname a certificate from Caddy's own local
certificate authority — real Let's Encrypt cannot issue for a
`*.localtest.me` name (it does not own that DNS and cannot complete a
challenge against `127.0.0.1`). A production deployment with real DNS names
removes the `tls internal` lines (or points them at real ACME) and gets
automatic Let's Encrypt certificates instead; nothing else changes.

Two consequences of "every origin is HTTPS" that this recipe wires up so the
stack is actually usable, not just bootable:

- **Ory Hydra trusts caddy's local CA.** `backchannel_logout_uri` is the
  public `https://…:8443` origin (Shauth rejects anything else), so Hydra —
  which delivers it server-to-server — needs to trust the certificate caddy
  presents. The `hydra` service mounts the `caddy-data` volume read-only and
  sets `SSL_CERT_FILE` to the root certificate caddy writes into it
  (`crypto/x509`'s Unix/Linux trust-store override — Hydra is a Go binary, so
  this needs no image change).
- **AWS/GCP/Azure simulators trust it too.** Verifying a federated operator's
  Shauth-issued assertion (`AssumeRoleWithWebIdentity`, a workforce pool
  provider, a federated identity credential's `client_assertion`) means
  fetching Shauth's real OpenID Connect discovery document and JWKS from
  `SHAUTH_PUBLIC_URL` — reached through caddy. `simulator-aws`,
  `simulator-gcp`, and `simulator-azure-cloud` all mount the same
  `caddy-data` volume and set the same `SSL_CERT_FILE`, so an actual
  browser-driven federation login (an operator signing in and using a
  console's credential-minting page) verifies correctly, not just the static
  resources `provision.sh` creates.

Both of these are Compose-only (`environment:`/`volumes:` on services this
recipe already owns) — no simulator, Shauth, or Hydra image was modified.
Because `caddy`'s local certificate authority is generated once and
persisted in the `caddy-data` volume, this is a one-time cold-start
concern: if the very first ever `sockerless login`/console federation
happens before caddy has produced its first certificate (a few seconds after
`caddy` starts), retry it — every subsequent boot reuses the same authority.

## `.env` schema and persistent origins

Copy `.env.example` to `.env` and fill in every value; `.env` and the
provisioning-time `.env.generated` are both git-ignored. The default origins
use the public `*.localtest.me` DNS zone (any subdomain resolves to
`127.0.0.1`, exactly where caddy publishes its port) combined with a Docker
Compose network alias per hostname *on the caddy service*, so a browser on
the host resolves e.g. `shauth.localtest.me` the same way a container on the
compose network does: Docker's embedded DNS answers a container's lookup
with caddy's own address before it ever reaches `localtest.me`'s public
records, while the host's real DNS resolver answers the browser with
`127.0.0.1`. A production deployment replaces every `*_HOSTNAME`/
`*_PUBLIC_URL` with a real DNS name pointed at wherever caddy (or an
equivalent real-ACME reverse proxy) runs — real DNS is symmetric the same
way, so nothing else about this file, `deploy/Caddyfile`, or the OIDC client
registrations `provision.sh` computes changes.

`deploy/.env.generated` (written by `provision.sh`, never by hand) carries
three coordinates that depend on other `.env` values or a provisioned cloud
resource: `HYDRA_DSN`/`SHAUTH_DATABASE_URL` (derived from
`POSTGRES_PASSWORD`, so the password has one source of truth),
`SHAUTH_BOOTSTRAP_APPS_JSON` (built from every `*_PUBLIC_URL` and
`*_OIDC_CLIENT_SECRET`), and `SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID` (the
Azure managed identity `provision.sh` mints — see "Boot order" below).

## Why two files, not an override

`compose.build.yaml` is a complete standalone file, not a `-f compose.yaml -f
compose.build.yaml` overlay. Docker Compose interpolates `${VAR:?err}`
per-file *before* merging multiple `-f` files, so even a `build:` override
in a second file cannot stop the first file's `${SOCKERLESS_TAG:?...}` from
demanding a value at parse time. Run exactly one of:

```sh
# Registry images (requires SOCKERLESS_TAG + SHAUTH_TAG in .env)
docker compose -f deploy/compose.yaml --env-file deploy/.env --env-file deploy/.env.generated up -d

# Build from source (requires SHAUTH_SOURCE_DIR — see .env.example)
docker compose -f deploy/compose.build.yaml --env-file deploy/.env --env-file deploy/.env.generated up -d
```

`provision.sh` runs the right one of these itself — see "Boot order".

## Boot order

Bringing the stack up is a two-pass sequence because
`simulator-azure-console` needs a managed-identity `clientId` that only
exists once `simulator-azure-cloud`'s Azure Resource Manager API has
provisioned it — the same ordering constraint BUG-2640 describes for the
relying-party harness, expressed here as a Compose profile:

1. **Boot the base profile + provision** — `deploy/provision.sh` computes
   `SHAUTH_BOOTSTRAP_APPS_JSON`, `HYDRA_DSN`, and `SHAUTH_DATABASE_URL` into
   `.env.generated`, brings up every service except
   `simulator-azure-console` (which Compose skips by default — it carries
   `profiles: ["console"]`) — including `caddy`, then extracts caddy's local
   certificate authority (`deploy/.caddy-local-ca.crt`, also git-ignored) so
   every following `curl` in the script trusts it via `CURL_CA_BUNDLE` — then
   waits for health, registers the `sockerless-cli` public Hydra client, and
   provisions AWS/GCP/Azure federation resources against the real simulator
   APIs (through caddy, using the same trusted certificate authority).
2. **Mint the Azure identity + boot the console profile** — still inside
   `provision.sh`: creates the Azure resource group, the
   `sockerless-console-identity` user-assigned managed identity, and a
   federated identity credential trusting the Shauth bootstrap
   administrator's subject; writes the identity's `clientId` to
   `SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID` in `.env.generated`; then runs
   `docker compose --profile console up -d simulator-azure-console` and
   waits for it to answer `/health`.

Run it:

```sh
cd deploy
cp .env.example .env   # fill in every value
DEPLOY_COMPOSE_FILE=compose.build.yaml ./provision.sh   # or compose.yaml for registry images
```

`provision.sh` is safe to re-run: every step either upserts (Shauth's own
bootstrap-apps reconciliation, ARM `PUT`) or explicitly tolerates the
cloud's own "already exists" response (AWS `EntityAlreadyExists`, GCP
`ALREADY_EXISTS`) before treating a non-2xx response as a real failure. A
full teardown-and-reboot needs a clean PostgreSQL volume if `.env`'s
`POSTGRES_PASSWORD` changed — Postgres only applies that variable on first
initialization of its data directory (see "Teardown").

## Provisioning in detail

- **Shauth OpenID Connect clients** — via Shauth's `SHAUTH_BOOTSTRAP_APPS_JSON`
  mechanism (reconciled on every Shauth startup, so recomputing this JSON and
  letting Compose recreate the `shauth` container is the whole update path):
  `sockerless-admin`, `sockerless-aws`, `sockerless-gcp`, and `sockerless-azure`
  (the **console** instance only — `simulator-azure-cloud` never serves a
  browser and is not a Shauth client). Every URL, including
  `backchannel_logout_uri`, is the public `https://…` origin through caddy —
  see "Why every origin is HTTPS".
- **`sockerless-cli`** — registered directly against Ory Hydra's admin API as
  a *public* client (`token_endpoint_auth_method: none`, PKCE, the RFC 8252
  loopback redirect `http://127.0.0.1/callback`). It is deliberately not a
  Shauth managed app: managed apps are browser applications with
  health/validation/logout-bridge URLs a terminal cannot serve, so the CLI
  runs Shauth's explicit consent screen once instead of managed-app
  auto-consent.
- **AWS** — an IAM OpenID Connect provider trusting the Shauth issuer, plus
  `console-federation-role` (the AWS console assumes it via
  `AssumeRoleWithWebIdentity`) and `cli-federation-role` (`sockerless login`
  assumes it the same way). Every call is signed with real SigV4 against the
  simulator's seeded bootstrap administrator credential (`test`/`test`,
  `us-east-1` — the same well-known coordinate every AWS SDK/CLI/Terraform
  test surface in this repository configures), because the AWS simulator
  verifies SigV4 on its control plane exactly as real AWS does. The trust
  policy document is built with `jq` rather than hand-escaped inline JSON in
  a bash string — a double-quoted string containing a backslash-escaped
  `[{...}]` array nested inside a `$(...)` command substitution trips a real
  parsing bug in bash 3.2 (macOS's default `/bin/bash`) that silently drops
  everything before the array.
- **Google Cloud** — a workforce pool (`sockerless-console`) with two OIDC
  providers: `sso` (audience `sockerless-gcp`, for the console) and `cli`
  (audience `sockerless-cli`, for `sockerless login`). Provisioning first
  mints a bootstrap access token from the simulator's own token endpoint
  (exempt from bearer enforcement, like real Google's own token endpoint)
  and presents it as the administrator bearer for the IAM calls that follow.
- **Microsoft Azure** — a resource group, the
  `sockerless-console-identity` user-assigned managed identity (its
  `clientId` is the coordinate `simulator-azure-console` federates as), and
  one federated identity credential trusting the Shauth bootstrap
  administrator's subject (audience `sockerless-azure`, matching the
  console's own Hydra client id — the assertion the federation broker
  presents is the operator's own console sign-in ID token). Provisioning
  authenticates to Azure Resource Manager with the simulator's seeded
  bootstrap service principal (`test-client-id`/`test-client-secret`, the
  same coordinate `the sockerless-cloud repository's simulator-azure/docs/terraform.md` documents), reached
  through caddy at `https://azure-cloud.localtest.me:8443`.

  **Real Azure constraint, not a sockerless limitation:** Microsoft Entra
  Workload Identity Federation pins federated identity credentials to one
  exact subject each (up to 20 per identity). Only the bootstrap
  administrator can use the Azure console's federation broker until an
  administrator provisions an additional federated identity credential
  (`PUT .../federatedIdentityCredentials/<name>` with that operator's Shauth
  user id as `subject`) for each additional operator — the same operation
  `provision.sh` performs for the bootstrap administrator, run again with a
  different subject.

## Smoke test

```sh
cd deploy
./smoke.sh
```

Checks, against the live origins (not in-process, not mocked):

- Every service answers `/health` or `/healthz`.
- Shauth's `/.well-known/openid-configuration` serves a discovery document
  with `issuer`, `authorization_endpoint`, `token_endpoint`, and `jwks_uri`.
- Every console's `/ui/` redirects an unauthenticated request to Shauth.
- Each simulator's data plane **rejects** an unauthenticated call (AWS
  `sts:GetCallerIdentity` → 403; GCP `workforcePools.get` → 401; Azure
  `resourceGroups.list` → 401) and **answers** the same call authenticated
  with the simulator's seeded credentials (AWS SigV4 `test`/`test`; GCP's
  bootstrap token minter; Azure's seeded service principal).
- `sockerless login --no-browser` prints a Shauth authorize URL; the script
  follows it (real HTTP redirects, no mocked transport) to confirm it
  resolves to Shauth's real login form. The CLI is built for `linux/$(go env
  GOARCH)` and run in a throwaway `alpine` container joined to the compose
  network, rather than run directly on the host: `crypto/x509`'s
  `SSL_CERT_FILE` override (needed to trust caddy's local certificate
  authority — see "Why every origin is HTTPS") is a Unix/Linux convention
  Darwin's root-of-trust implementation does not consult at all, so a
  macOS-built/run binary can never trust a locally-issued certificate no
  matter how it is invoked; a Linux container reaching the same compose
  network exercises exactly the code path a real Linux deployment host (or
  this repository's own Linux CI) uses natively, on every host OS this
  script itself runs on.

`smoke.sh` does not exercise a full interactive login (that is Shauth's own
relying-party matrix, `scripts/test-shauth-rps.sh`) — it proves the deployed
CLI reaches a real, live authorize endpoint at the coordinates this
deployment advertises.

## Upgrade

Registry-image deployments: publish a new commit (which mints a new
`SOCKERLESS_TAG`), set `SOCKERLESS_TAG` in `.env` to the new short SHA, then:

```sh
docker compose -f compose.yaml --env-file .env --env-file .env.generated pull
docker compose -f compose.yaml --env-file .env --env-file .env.generated up -d
```

`APPLICATION_RELEASE_REVISION` should track `SOCKERLESS_TAG` so Shauth's
release-identity validation and each console's monitoring page report the
running revision accurately. Bump `SHAUTH_TAG` (and re-run `provision.sh` to
refresh `SHAUTH_BOOTSTRAP_APPS_JSON`'s `release_revision` field) the same way
when Shauth's own pinned commit moves.

## Teardown

```sh
docker compose -f compose.yaml --env-file .env --env-file .env.generated --profile console down --volumes
```

The `--profile console` flag matters: `simulator-azure-console` carries
`profiles: ["console"]`, so a plain `down` (without it) leaves that one
container running. This removes the PostgreSQL volume (all Shauth
identity/session state) and the `caddy-data` volume (caddy's local
certificate authority — a fresh one regenerates on next boot, and every
`SSL_CERT_FILE`-trusting service picks it up the same way) along with every
container. Federation resources provisioned against the simulators (IAM
  role, workforce pool, managed identity) live in the simulators' named
  SQLite volumes and survive ordinary container replacement. The explicit
  `down --volumes` command above deletes those cloud-state volumes as well as
  the identity and certificate-authority volumes. A real-cloud deployment's
  federation resources are Terraform- or
API-provisioned infrastructure outside this compose stack's lifecycle and
are not touched by `docker compose down`.

Changing `POSTGRES_PASSWORD` in `.env` requires this full teardown first:
PostgreSQL only applies that variable when initializing an empty data
directory, so an existing `postgres-data` volume keeps its original password
and every dependent service (Hydra, Shauth) fails to authenticate against
the `HYDRA_DSN`/`SHAUTH_DATABASE_URL` `provision.sh` derives from the new one.

## Security

- **Secrets** — every value in `.env`'s "Secrets" section must be generated
  (`openssl rand -hex 32` / `openssl rand -base64 48`), never reused across
  environments, and never committed. `.env`, `.env.generated`, and the
  extracted `.caddy-local-ca.crt` are all git-ignored; `.env.example`
  documents the schema with empty values only.
- **TLS termination** — `caddy` is the only service that publishes a host
  port (bound to `127.0.0.1`) and the only one a production deployment needs
  to expose publicly; every backend service is reachable exclusively through
  it. See "Why every origin is HTTPS" for what a production deployment (real
  DNS, real ACME certificates) changes in `deploy/Caddyfile` and what it does
  not. `SHAUTH_INSECURE_COOKIES` defaults to `false` because every origin is
  HTTPS by default — only set it `true` if `deploy/Caddyfile` has been
  reconfigured to serve plain HTTP over the literal hostname `localhost`
  instead (see `cmd/sockerless-admin/shauth.go`'s `validate()` for exactly
  what that requires).
- **Ory Hydra admin API** — `HYDRA_PUBLIC_PORT`/`HYDRA_ADMIN_PORT` are bound
  to `127.0.0.1` only and are not fronted by caddy. Hydra's public OIDC
  endpoints are reached exclusively through Shauth's own reverse-proxying
  (`SHAUTH_PUBLIC_URL` == `HYDRA_PUBLIC_URL`); the admin API (`4445`) is a
  bearer-less administrative surface that must never be reachable from
  outside the deployment host or its private network.
- **PostgreSQL** — bound to `127.0.0.1` only, for operator debugging
  (`psql`); a production deployment should instead put it on a private
  network with no host port published at all.
- **Shauth admin bootstrap password** — `SHAUTH_BOOTSTRAP_ADMIN_PASSWORD`
  creates the one local account (`SHAUTH_BOOTSTRAP_ADMIN_EMAIL`, default
  `admin@localhost.test`) that exists before any GitHub sign-in is
  configured. Rotate it after first boot the same way any other Shauth local
  account password is rotated (Shauth's own account settings page), and
  treat the `.env` value as a one-time bootstrap secret, not a long-lived
  operator credential.
- **GitHub OAuth application** — `GITHUB_CLIENT_ID`/`GITHUB_CLIENT_SECRET`
  are Shauth's upstream sign-in option; `GITHUB_DEVELOPER_TEAM`/
  `GITHUB_ADMIN_TEAM` gate the developer/admin role Shauth grants to members
  of those GitHub teams. The placeholder values in `.env.example` let the
  stack boot without a real GitHub OAuth App (Shauth requires all four
  non-empty), but GitHub sign-in will not function until they are replaced
  with a real OAuth App's credentials and your organization's team slugs.
- **Azure federated identity credentials are per-operator** — see
  "Provisioning in detail" above; do not work around the one-subject-per-FIC
  constraint with a shared credential or a wildcard subject — Microsoft
  Entra does not offer either, and the simulator faithfully does not either.
- **`SIM_RUNTIME=process`** — every simulator runs in API-only mode (no
  Docker/Podman socket is mounted into any simulator container), because
  this recipe's purpose is Admin + the three consoles + identity, not
  executing ECS/Lambda-class/Cloud Run/Azure Functions-style container
  workloads. A deployment that also wants to exercise backend workload
  execution mounts a real Docker/Podman socket into the relevant simulator
  and removes `SIM_RUNTIME: process` from its environment (the default is
  `docker`) — see each simulator's `main.go` doc comment.

## Coordinate → consumer map

| Coordinate | Set by | Read by |
|---|---|---|
| `SHAUTH_PUBLIC_URL` / `HYDRA_PUBLIC_URL` | `.env` | every relying party's `SIM_UI_OIDC_ISSUER` / `SOCKERLESS_ADMIN_SHAUTH_ISSUER`; `provision.sh`'s OIDC-provider and workforce-provider `issuerUri`; Azure FIC `issuer`; every simulator's outbound JWKS fetch |
| `ADMIN_PUBLIC_URL`, `AWS_SIM_PUBLIC_URL`, `GCP_SIM_PUBLIC_URL`, `AZURE_CONSOLE_SIM_PUBLIC_URL` | `.env` | that service's own `*_PUBLIC_URL`/`SIM_UI_PUBLIC_URL`; `provision.sh`'s bootstrap-apps `redirect_uris`/`*_logout_uri`/`health_url`/`validation_url` (all through caddy) |
| `*_HOSTNAME` (six of them) | `.env` | caddy's network aliases + `deploy/Caddyfile`'s site addresses; `AZURE_CLOUD_SIM_HOSTNAME` also feeds `provision.sh`'s and `smoke.sh`'s direct ARM/token calls |
| `CADDY_PORT` | `.env` | `deploy/Caddyfile`; every `*_PUBLIC_URL` |
| `AZURE_FEDERATION_TENANT` | `.env` | `simulator-azure-console`'s `SOCKERLESS_CONSOLE_FEDERATION_TENANT`; `provision.sh`'s and `smoke.sh`'s Entra token-endpoint path |
| `SOCKERLESS_CONSOLE_FEDERATION_CLIENT_ID` | `provision.sh` → `.env.generated` | `simulator-azure-console` only |
| `HYDRA_DSN`, `SHAUTH_DATABASE_URL` | `provision.sh` → `.env.generated` (derived from `POSTGRES_PASSWORD`) | `hydra`/`hydra-migrate`, `shauth`/`shauth-migrate` |
| `SHAUTH_BOOTSTRAP_APPS_JSON` | `provision.sh` → `.env.generated` | `shauth` only |
| caddy's local certificate authority (`caddy-data` volume) | `caddy` (generated at first boot, persisted) | `hydra`, `simulator-aws`, `simulator-gcp`, `simulator-azure-cloud` (mounted read-only, referenced via `SSL_CERT_FILE`); `provision.sh`/`smoke.sh` (extracted to `.caddy-local-ca.crt`, referenced via `CURL_CA_BUNDLE`) |
| `ADMIN_OIDC_CLIENT_SECRET`, `AWS_OIDC_CLIENT_SECRET`, `GCP_OIDC_CLIENT_SECRET`, `AZURE_OIDC_CLIENT_SECRET` | `.env` | that service's own `*_CLIENT_SECRET` env **and** `provision.sh`'s bootstrap-apps `oidc_client_secret` — one value, two consumers, never duplicated by hand beyond `.env` itself |
| `AWS_FEDERATION_ROLE_ARN`, `GCP_WORKFORCE_PROVIDER` | `.env` (documented default matching what `provision.sh` provisions) | `simulator-aws`/`simulator-gcp`'s `SOCKERLESS_CONSOLE_FEDERATION_AUDIENCE` |

## Continuous integration

No CI job runs this recipe — timed evidence below shows it cannot fit the
repository's 15-minute-per-job budget (`AGENTS.md` "Always fix CI failures",
every workflow's `timeout-minutes: 15`). Run it manually with `make
deploy-smoke-build` (source build) or `make deploy-smoke` (registry images,
needs `SOCKERLESS_TAG`) before releasing this phase and before any change to
`deploy/**`; `make deploy-down` tears it down.

**Timed evidence** (this repository's own build machine, 12 vCPUs, Podman
6.0.1 machine, `docker compose build` targeting `compose.build.yaml`):

| Scenario | Wall time |
|---|---|
| One image (`simulator-azure`), fully cold — no BuildKit layer cache, base images (`golang`, `oven/bun`) already present locally | 2m52s |
| All five images (`sockerless-admin` + 3 simulators + Shauth/Hydra), BuildKit layer cache pruned (`docker builder prune -af`) but base images present, `docker compose build`'s own parallelism across 12 cores | **10m07s** (`time docker compose -f compose.build.yaml build`) |
| `provision.sh` + `smoke.sh` against already-built images (fresh containers/volumes each time) | well under a minute combined |

A GitHub Actions runner has 4 vCPUs (a fraction of the parallelism above),
starts every job with no local image cache at all (`golang`, `oven/bun`,
`caddy`, `postgres` all pull from scratch), and would additionally need to
check out the pinned Shauth commit and build its own two images (`shauth`,
`hydra`, plus patching and downloading Hydra's source). The build phase alone
is evidenced to meaningfully exceed 15 minutes before provisioning or smoke
even start — adding a CI job for it would either need a much larger
timeout (against this repository's own bounded-automation rule) or would be
flaky/red by construction. A `type=gha` BuildKit cache (as
`publish-container-images.yml` already uses per image) would help a
*second* run, but the first run on any new cache scope is exactly the cold
case measured above, and this recipe changes rarely enough that the cache
would routinely be cold in practice.
