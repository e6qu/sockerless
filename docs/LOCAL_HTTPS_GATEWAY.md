# Local HTTPS Gateway

Sockerless can run an optional Caddy front door for local simulator APIs. The simulators still listen on their normal HTTP loopback ports; Caddy terminates HTTPS and reverse-proxies to them.

This is local transport infrastructure. It does not add simulator-specific cloud API routes, headers, request fields, or response shapes.

Relevant Caddy references: [Caddyfile environment substitutions](https://caddyserver.com/docs/caddyfile/concepts), [`tls internal`](https://caddyserver.com/docs/caddyfile/directives/tls), and [`reverse_proxy`](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy).

## Why It Exists

Terraform provider endpoint behavior differs by cloud:

- AzureRM requires trusted HTTPS for custom metadata discovery because `metadata_host` is host-only and the provider builds `https://<host>`.
- Azure Stack is HTTPS-shaped for ARM and metadata use.
- AzAPI accepts full endpoint URLs and defaults to HTTPS Azure endpoints.
- AWS and GCP providers accept full custom endpoint URLs; existing HTTP localhost endpoints remain valid.

The gateway is therefore most important for Azure Terraform and SDK paths that validate HTTPS endpoint URLs. AWS and GCP can use it for parity with public cloud URLs, but do not require it for the current simulator Terraform paths.

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
| Azure ARM/metadata | `https://azure.sockerless.localhost:8443` |
| Azure Blob | `https://{account}.blob.azure.sockerless.localhost:8443` |
| Azure Key Vault | `https://{vault}.vault.azure.sockerless.localhost:8443` |
| Azure Service Bus | `https://{namespace}.servicebus.azure.sockerless.localhost:8443` |

Override the port with:

```sh
make stack-https-up STACK_HTTPS_PORT=9443
```

## CA Trust

Caddy creates a local CA under the gateway state directory. Print the CA path with:

```sh
make stack-https-ca
```

Linux test containers can trust it with:

```sh
export SSL_CERT_FILE="$(make -s stack-https-ca)"
```

macOS system trust is separate from Go's Linux `SSL_CERT_FILE` path. For provider tests that must honor `SSL_CERT_FILE`, keep using the Linux Docker harness.

## Stack Integration

To start a normal dev stack and the HTTPS gateway together:

```sh
STACK_HTTPS=1 make stack-azure-aca
```

For Azure stacks, this also configures `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` so ARM responses advertise host-addressed HTTPS data-plane endpoints under `azure.sockerless.localhost`.

Direct simulator TLS remains available through `SIM_TLS_CERT` and `SIM_TLS_KEY`. Use that lower-level path when a test needs to exercise a simulator's own TLS listener rather than the local gateway.
