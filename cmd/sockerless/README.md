# sockerless (CLI)

Zero-dependency CLI for managing Sockerless contexts and backend server lifecycle. Stores configuration in `~/.sockerless/` (override with `SOCKERLESS_HOME`). Talks to backends over their Docker REST API + management endpoints.

## Reference adaptors

The CLI is itself an adaptor against running Sockerless backends and against the user's shell. Its surface is small and validated by:

| Adaptor | What it proves |
|---|---|
| The user's terminal | `sockerless context`, `sockerless server`, `sockerless ps`, `sockerless status` — typed commands round-trip without surprises. |
| The backend management HTTP API | `/v1/health`, `/v1/info`, `/v1/containers`, `/v1/metrics` on each [backend](../../backends/) at the configured `addr`. The CLI is a thin HTTP client; backends are the truth. |
| The [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/) | `sockerless ps` parses `/containers/json`; `sockerless metrics` reads Prometheus over HTTP. |
| The `aws` / `gcloud` / `az` CLIs | `sockerless login` writes the configuration those tools refresh themselves (an `~/.aws/config` profile, a workforce Application Default Credentials file activated with `gcloud auth login --cred-file`, an `az login --federated-token` session) and `sockerless logout` removes it. |
| `*_test.go` files in this package | Behaviour-level unit tests for context CRUD, config-migrate, simulator add/remove, status formatting, the OpenID Connect + PKCE login flow, and the INI-preserving `~/.aws/config` editing. |

This means `sockerless` does **not** speak any cloud API directly. Configuration *describes* cloud backends; the cloud calls happen inside the backend processes. `sockerless login` follows the same rule: it signs in to Shauth (an OpenID Connect issuer), then hands the identity token to the vendor tools — the aws CLI, gcloud, and az run each cloud's federation exchange themselves.

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `cmd/sockerless/*_test.go` | Go unit tests for context store, config migrate, simulator manager, paths. | 2026-05-16 |
| `make cmd/sockerless/test` | Leaf-Makefile suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-16 |
| Manual round-trip | `sockerless context create … && sockerless server start && sockerless status` against a built backend binary. Discipline: real binary, real terminal output — see [`manual-test`](../../.claude/skills/manual-test/SKILL.md). | continuous |

## Wiring

```sh
# Build
make cmd/sockerless/build

# Initialise a context backed by ECS + the AWS sim
sockerless context create ecs-dev --backend ecs --simulator sim-aws \
  --set SOCKERLESS_ECS_CLUSTER=sockerless \
  --set SOCKERLESS_ECS_SUBNETS=subnet-abc123 \
  --set SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/exec

# Start the backend
sockerless server start

# Drive workloads via the Docker frontend
export DOCKER_HOST=tcp://localhost:3375
docker run --rm alpine echo hello
```

### Environment variables

| Variable | Description |
|---|---|
| `SOCKERLESS_HOME` | Override config directory (default `~/.sockerless`). |
| `SOCKERLESS_CONTEXT` | Override active context name. |
| `SOCKERLESS_CONFIG` | Override config file path (default `~/.sockerless/config.yaml`). |

## Commands

### `context` — manage backend contexts

```sh
sockerless context create myctx --backend ecs
sockerless context create aws-dev --backend ecs --set AWS_REGION=us-east-1
sockerless context create sim-ecs --backend ecs --simulator sim-aws
sockerless context list
sockerless context show myctx
sockerless context use myctx
sockerless context current
sockerless context delete myctx
sockerless context reload
```

Flags for `context create`:

| Flag | Description |
|---|---|
| `--backend` | Backend type (required): `ecs`, `lambda`, `cloudrun`, `gcf`, `aca`, `azf`, `docker` |
| `--addr` | Server address (e.g. `http://localhost:3375`) |
| `--simulator` | Simulator name (from `config.yaml` simulators section) |
| `--set KEY=VALUE` | Set environment variable (repeatable) |

### `server` — lifecycle

```sh
sockerless server start
sockerless server stop
sockerless server restart
```

Flags for `server start`:

| Flag | Default | Description |
|---|---|---|
| `--backend-bin` | `sockerless-backend-{type}` | Path to backend binary |
| `--addr` | `:3375` | Listen address (Docker API + management) |

### Inspection

```sh
sockerless status      # Backend health + uptime + container count
sockerless ps          # Table: ID, NAME, IMAGE, STATE, POD
sockerless metrics     # Prometheus metrics from the backend
sockerless check       # Backend self-checks with per-check pass/fail
sockerless resources list      # Cloud resources owned by this backend
sockerless resources orphaned  # Resources without a matching sockerless owner-link
sockerless resources cleanup   # Reap orphans
```

### `simulator` — manage local cloud simulators

```sh
sockerless simulator list
sockerless simulator add sim-aws --cloud aws --port 5111
sockerless simulator add sim-gcp --cloud gcp --port 5112 --grpc-port 5113
sockerless simulator remove sim-aws
```

Flags for `simulator add`:

| Flag | Description |
|---|---|
| `--cloud` | Cloud type (required): `aws`, `gcp`, `azure` |
| `--port` | Listen port (0 = auto) |
| `--grpc-port` | gRPC port (GCP only) |
| `--log-level` | Log level |

### `login` / `logout` — Shauth sign-in for the vendor CLIs

`sockerless login` is the packaged terminal analog of `aws configure sso` /
`gcloud auth login` / `az login`: one browser sign-in to Shauth, and the
unmodified vendor CLIs and SDKs work against the configured clouds or
deployed simulators. The flow is the RFC 8252 native-app flow — OpenID
Connect authorization code with PKCE (S256) and an ephemeral loopback
redirect on `127.0.0.1`.

```sh
sockerless login                 # sign in, wire every configured cloud
sockerless login --cloud aws     # limit to one cloud (aws, gcp, or azure)
sockerless login --no-browser    # print the sign-in URL instead of opening a browser
sockerless login --timeout 2m    # how long to wait for the browser (default 5m)
sockerless logout                # remove everything login wrote
```

Coordinates live in the active context's `config.yaml` environment. A cloud
without its login coordinates is skipped loudly — absence is configuration:

```yaml
environments:
  dev:
    backend: ecs
    login:
      issuer: https://shauth.example.com     # Shauth OpenID Connect issuer
      client_id: sockerless-cli              # public client; client_secret only for confidential clients
    aws:
      region: us-east-1
      login:
        role_arn: arn:aws:iam::123456789012:role/cli-federation-role
        endpoint_url: http://localhost:29310 # omit to target real AWS
        # profile: sockerless-dev            # optional; default sockerless-<context>
    gcp:
      project: my-project
      login:
        workforce_audience: //iam.googleapis.com/locations/global/workforcePools/pool/providers/cli
        sts_endpoint: http://localhost:29320 # omit for https://sts.googleapis.com
        api_endpoint: http://localhost:29320 # omit to leave gcloud pointed at real Google
        # workforce_pool_user_project: p     # optional; defaults to gcp.project
        # configuration: sockerless-dev      # optional gcloud configuration name
    azure:
      subscription_id: 00000000-0000-0000-0000-000000000001
      login:
        tenant: 11111111-1111-1111-1111-111111111111
        client_id: 33333333-3333-3333-3333-333333333333  # federation client (user-assigned identity)
        authority_endpoint: https://sim.example.com:29331 # omit both endpoints for the Azure public cloud
        resource_manager_endpoint: https://sim.example.com:29331
        ca_bundle: /path/to/sim-tls.crt      # az reads it via REQUESTS_CA_BUNDLE
        # cloud_name: sockerless-dev         # optional az cloud name
```

What login writes per cloud — always configuration the vendor tools refresh
themselves, never one-shot copied secrets:

- **Shauth identity token** →
  `~/.sockerless/contexts/<ctx>/web-identity-token` (0600). Every cloud's
  federation reads it. It expires on the issuer's ID-token lifetime; run
  `sockerless login` again after that.
- **Amazon Web Services** → a `[profile sockerless-<ctx>]` section in
  `~/.aws/config` (`AWS_CONFIG_FILE` honored) with `role_arn`,
  `web_identity_token_file`, `region`, and `endpoint_url`. The aws CLI and
  SDKs run `AssumeRoleWithWebIdentity` themselves on demand. Only that one
  section is touched — the rest of the file is preserved byte-for-byte.
  Verify: `aws --profile sockerless-<ctx> sts get-caller-identity`.
- **Google Cloud** → a Workforce Identity Federation `external_account`
  Application Default Credentials file at
  `~/.sockerless/contexts/<ctx>/gcp-adc.json` (`token_url` /
  `token_info_url` at the Security Token Service coordinate,
  `credential_source.file` naming the token file), activated in a dedicated
  gcloud configuration (`sockerless-<ctx>`, created with `--no-activate`)
  via `gcloud auth login --cred-file`; with `api_endpoint` set, the
  configuration also carries the `api_endpoint_overrides/*` properties the
  simulator CLI test suite proves. SDKs use
  `GOOGLE_APPLICATION_CREDENTIALS=<the adc file>`.
  Verify: `gcloud --configuration=sockerless-<ctx> projects list`.
- **Microsoft Azure** → `az login --service-principal --federated-token`
  for the federation client; az stores the assertion in its own token cache
  and re-exchanges it itself until the assertion expires. With simulator
  endpoints configured, login first registers/updates and selects an az
  cloud (`az cloud register/update` + `az cloud set`) and disables MSAL
  instance discovery (`az config set core.instance_discovery=false`) — the
  az CLI requires HTTPS coordinates, so a simulator target must serve TLS
  and `ca_bundle` names its trust bundle.
  Verify: `az account show`.

If `gcloud` or `az` is missing from `PATH`, login fails loudly for that
cloud and prints the exact commands to run after installing it (the aws
wiring needs no aws CLI — it only writes files).

`sockerless logout` removes the token file, the Application Default
Credentials file, and the `[profile sockerless-<ctx>]` section, and runs
`az logout` when az is installed. It does not switch the active az cloud
back (`az cloud set -n AzureCloud`) or delete the gcloud configuration
(`gcloud config configurations delete sockerless-<ctx>`) — those remain as
inert named configurations.

### `config migrate` — convert JSON contexts to `config.yaml`

```sh
sockerless config migrate          # preview to stdout
sockerless config migrate --write  # write to config.yaml
```

Reads existing `contexts/*/config.json` files and converts them to the unified `config.yaml` format. Detects simulator usage from `SOCKERLESS_ENDPOINT_URL` and creates simulator entries automatically.

### `version`

```sh
sockerless version
```

## Configuration layout

```
~/.sockerless/
├── config.yaml          Unified configuration (environments + simulators)
├── active               Active context/environment name
├── contexts/            Legacy JSON contexts (still supported)
│   └── {name}/
│       └── config.json  Backend type, address, env vars
└── run/
    └── {name}/
        └── backend.pid  Server process ID
```

## Unified configuration

The preferred way to configure Sockerless is via `~/.sockerless/config.yaml`. This file defines named environments (backend configurations) and optional simulator definitions in a single place.

```yaml
simulators:
  sim-aws:
    cloud: aws
    port: 5111
  sim-gcp:
    cloud: gcp
    port: 5112
    grpc_port: 5113

environments:
  ecs-sim:
    backend: ecs
    simulator: sim-aws
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-abc123]
        execution_role_arn: arn:aws:iam::123456789012:role/ecsExec
  cloudrun-dev:
    backend: cloudrun
    simulator: sim-gcp
    gcp:
      project: my-project
      cloudrun:
        region: us-central1
  aca-prod:
    backend: aca
    azure:
      subscription_id: 00000000-0000-0000-0000-000000000000
      aca:
        resource_group: sockerless-rg
        environment: sockerless
        location: eastus
    common:
      agent_image: myregistry.azurecr.io/sockerless-agent:latest
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

**Priority order:** `config.yaml` environment values → context env vars (legacy) → process environment variables → defaults.

Use `sockerless config migrate` to convert existing JSON contexts to `config.yaml` format. Legacy `contexts/*/config.json` files continue to work — the CLI checks `config.yaml` first, then falls back to JSON contexts.

## Known issues

None open. CLI evolution tracked alongside backend evolution in [`PLAN.md`](../../PLAN.md).

## What's out of scope

- Cloud-API operations. The CLI configures backends; it doesn't itself call AWS / GCP / Azure. Use `aws` / `gcloud` / `az` for cloud-side observation — `sockerless login` exists precisely to make those tools work against the configured coordinates.
- Container exec / attach UX. Use the Docker frontend (`docker exec`, `docker attach`) — the backend serves the Docker REST API.
- Multi-machine orchestration. Use `cmd/sockerless-admin` instead — that surface is dedicated to topology.

## Project structure

```
cmd/sockerless/
├── main.go            Command dispatcher
├── configfile.go      Unified config.yaml types and I/O
├── config_migrate.go  JSON context → config.yaml migration
├── simulator.go       Simulator list/add/remove commands
├── context.go         Context CRUD commands
├── login.go           login/logout commands + per-cloud credential wiring
├── login_oidc.go      OpenID Connect authorization-code + PKCE loopback flow
├── login_awsconfig.go INI-preserving ~/.aws/config profile editing
├── server.go          Server start/stop/restart
├── status.go          Health status display
├── ps.go              Container listing
├── metrics.go         Metrics display
├── resources.go       Cloud resource management
├── check.go           Health check runner
├── client.go          HTTP management client helpers
└── paths.go           Config directory and path resolution
```

See also: [`backends/*/README.md`](../../backends/) for what each backend type configures, [`cmd/sockerless-admin/README.md`](../sockerless-admin/README.md) for the topology / multi-backend orchestration surface, [`specs/CONFIG.md`](../../specs/CONFIG.md) for the full configuration schema.
