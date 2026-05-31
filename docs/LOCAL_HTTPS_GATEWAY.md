# Local HTTPS Gateway

Sockerless can run an optional Caddy front door for local simulator APIs. The simulators still listen on their normal HTTP loopback ports; Caddy terminates HTTPS and reverse-proxies to them.

This is local transport infrastructure. It does not add simulator-specific cloud API routes, headers, request fields, or response shapes.

Relevant Caddy references: [Caddyfile environment substitutions](https://caddyserver.com/docs/caddyfile/concepts), [`tls internal`](https://caddyserver.com/docs/caddyfile/directives/tls), [`skip_install_trust`](https://caddyserver.com/docs/caddyfile/options#skip-install-trust), and [`reverse_proxy`](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy).

## Why It Exists

Terraform provider endpoint behavior differs by cloud:

- AzureRM requires trusted HTTPS for custom metadata discovery because `metadata_host` is host-only and the provider builds `https://<host>`.
- Azure Stack is HTTPS-shaped for ARM and metadata use.
- AzAPI accepts full endpoint URLs and defaults to HTTPS Azure endpoints.
- AWS and GCP providers accept full custom endpoint URLs; existing HTTP localhost endpoints remain valid.

The gateway is therefore most important for Azure Terraform and SDK paths that validate HTTPS endpoint URLs. AWS and GCP can use it for parity with public cloud URLs, but do not require it for the current simulator Terraform paths.

The Terraform CI path keeps Azure on this gateway because AzureRM metadata discovery requires trusted HTTPS. AWS and GCP also have HTTPS gateway harness targets so their providers are periodically exercised against public-cloud-shaped HTTPS URLs while their direct HTTP endpoint overrides remain available for local development.

## Commands

```sh
make stack-https-up
make stack-https-status
make stack-https-ca
make stack-https-down
```

The gateway uses the repo's normal stack bookkeeping:

- PID: `.stack-pids/https-gateway.pid`
- Log: `.stack-pids/https-gateway.log`
- Env record: `.stack-pids/https-gateway.env`
- Caddy state and local CA: `.sockerless-state/https-gateway/`

`make stack-down` stops the gateway because it is a normal `.stack-pids/*.pid` process.

## Endpoints

Default HTTPS port: `8443`.

| Cloud | Endpoint |
|---|---|
| AWS | `https://aws.sockerless.localhost:8443` |
| GCP | `https://gcp.sockerless.localhost:8443` |
| Single-simulator localhost route | `https://localhost:8443` |
| Azure ARM/metadata | `https://azure.sockerless.localhost:8443` |
| Azure Blob | `https://{account}.blob.azure.sockerless.localhost:8443` |
| Azure Files | `https://{account}.file.azure.sockerless.localhost:8443` |
| Azure Queue | `https://{account}.queue.azure.sockerless.localhost:8443` |
| Azure Table | `https://{account}.table.azure.sockerless.localhost:8443` |
| Azure Data Lake Storage | `https://{account}.dfs.azure.sockerless.localhost:8443` |
| Azure Key Vault | `https://{vault}.vault.azure.sockerless.localhost:8443` |
| Azure Service Bus | `https://{namespace}.servicebus.azure.sockerless.localhost:8443` |
| Azure Event Grid | `https://{topic}.eventgrid.azure.sockerless.localhost:8443` |
| Azure Cosmos DB documents | `https://{account}.documents.azure.sockerless.localhost:8443` |

Override the port with:

```sh
make stack-https-up STACK_HTTPS_PORT=9443
```

The `localhost` route is for resolver-independent single-simulator tests and points to `SOCKERLESS_HTTPS_GATEWAY_DEFAULT_SIM_PORT`, which the normal stack sets to the AWS simulator port. Public, cloud-shaped gateway names remain the named `*.sockerless.localhost` endpoints above.

## CA Trust

Caddy creates a local CA under the gateway state directory. Print the CA path with:

```sh
make stack-https-ca
```

Linux test containers can trust it with:

```sh
export SSL_CERT_FILE="$(make -s stack-https-ca)"
```

The gateway deliberately sets Caddy's `skip_install_trust` option. Tests and developer commands trust the exported CA file explicitly instead of letting Caddy mutate host, CI runner, Java, or browser trust stores. This keeps gateway startup non-interactive and keeps TLS verification enabled.

If a developer shell has proxy variables set, ensure local simulator hosts bypass the proxy:

```sh
export NO_PROXY="localhost,127.0.0.1,::1,sockerless.localhost,.sockerless.localhost,*.sockerless.localhost${NO_PROXY:+,$NO_PROXY}"
```

macOS system trust is separate from Go's Linux `SSL_CERT_FILE` path. For provider tests that must honor `SSL_CERT_FILE`, keep using the Linux Docker harness.

## SDK and CLI Clients

For AWS CLI, use `AWS_ENDPOINT_URL` or `--endpoint-url` with the gateway URL, and use `AWS_CA_BUNDLE` or `--ca-bundle` for the Caddy CA. AWS documents both endpoint overrides and CA bundle configuration in the CLI reference.

For AWS SDKs, use the same custom endpoint support already used for the direct simulator URL. SDKs and tools that honor the shared AWS configuration can also use `AWS_CA_BUNDLE`.

For gcloud, use the same `CLOUDSDK_API_ENDPOINT_OVERRIDES_*` variables as the direct simulator path, but point them at the gateway URL. Current gcloud documentation includes `CLOUDSDK_API_ENDPOINT_OVERRIDES_STORAGE` for `gcloud storage`, and `core/custom_ca_certs_file` is the documented custom CA property.

For Google Cloud Go clients, keep using explicit endpoint options such as `option.WithEndpoint` or emulator variables such as `STORAGE_EMULATOR_HOST` where the library supports them. HTTPS gateway clients must trust the Caddy CA through the process trust store or an HTTP client configured with that root.

For Azure CLI `az rest`, pass full gateway URLs. Use a trusted CA path such as `REQUESTS_CA_BUNDLE` for Python-request based CLI paths rather than disabling verification.

For Azure SDKs, use the same per-client endpoint/base URL override used for the direct simulator. HTTPS gateway clients must trust the Caddy CA through the runtime trust store or the SDK transport's root CA configuration.

Reference docs: [AWS CLI endpoint overrides](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-endpoints.html), [AWS CLI CA bundle](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-options.html), [gcloud endpoint overrides](https://docs.cloud.google.com/sdk/gcloud/reference/topic/endpoint-override), [gcloud custom CA property](https://docs.cloud.google.com/sdk/gcloud/reference/topic/configurations), [Azure CLI `az rest`](https://learn.microsoft.com/en-us/cli/azure/reference-index?view=azure-cli-latest#az-rest).

## Terraform Providers

Azure Terraform uses Caddy HTTPS as the canonical test path. The AzureRM and Azure Stack providers build HTTPS metadata URLs from a host-only `metadata_host`, so tests fail loudly when Caddy or HTTPS is missing.

AWS and GCP Terraform providers accept full custom endpoint URLs. Their direct HTTP configs stay valid, and the optional HTTPS configs use the same provider endpoint fields with the gateway base URL:

```sh
cd simulators/aws
make terraform-https-test

cd simulators/gcp
make terraform-https-test
```

Those targets start the simulator on HTTP loopback, put Caddy in front of it, set `SSL_CERT_FILE` to Caddy's local CA, and run the real Terraform provider apply/destroy harness through the gateway's `https://localhost:<ephemeral-port>` single-simulator route. On macOS they run inside the shared Linux simulator test image, matching the Azure Terraform pattern, because the real providers use Go's macOS trust integration and do not reliably honor `SSL_CERT_FILE` from the shell. That keeps the test independent of workstation or CI wildcard `.localhost` resolver behavior while still exercising real HTTPS, Caddy, CA validation, and the official providers.

## Stack Integration

To start a normal dev stack and the HTTPS gateway together:

```sh
STACK_HTTPS=1 make stack-azure-aca
```

For Azure stacks, this also configures `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` so ARM responses advertise host-addressed HTTPS data-plane endpoints under `azure.sockerless.localhost`.

Direct simulator TLS remains available through `SIM_TLS_CERT` and `SIM_TLS_KEY`. Use that lower-level path when a test needs to exercise a simulator's own TLS listener rather than the local gateway.
