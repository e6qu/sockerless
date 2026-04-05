# Docker Backend

Passes all Docker API calls through to a local Docker daemon (Docker Desktop, Colima, etc.).

## Config (config.yaml)

```yaml
environments:
  local:
    backend: docker
    addr: ":9100"
    log_level: info
```

The Docker backend has no cloud-specific configuration. It connects to the local Docker socket.

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `DOCKER_HOST` | auto-detect | no | Docker daemon socket (e.g. `unix:///var/run/docker.sock`) |
| `DOCKER_TLS_VERIFY` | | no | Enable TLS verification for Docker daemon |
| `DOCKER_CERT_PATH` | | no | Path to TLS certificates for Docker daemon |

All standard Docker client environment variables are respected.

## Quick Start

```sh
go build -o sockerless-backend-docker ./backends/docker/cmd
./sockerless-backend-docker -addr :9100 -log-level info
```

Flags: `-addr` (default `:9100`), `-docker-host` (default auto-detect), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Notes

- Requires a running Docker daemon accessible via the socket.
- This backend is a thin passthrough -- all operations are delegated directly to Docker.
- No agent sidecar is involved; exec, attach, and logs go straight to the Docker daemon.
- The `-docker-host` flag overrides the `DOCKER_HOST` environment variable.
- Useful for local development and testing without any cloud infrastructure.
- See `specs/CONFIG.md` for the full unified config specification.
