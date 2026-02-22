# sockerless-agent

A WebSocket-based remote execution agent for Sockerless. It runs inside containers (or as a sidecar) and provides multiplexed command execution and process attachment over a single WebSocket connection.

## Overview

The agent operates in two modes:

- **Server mode** — Listens on a port for incoming WebSocket connections from a Sockerless backend. Used by long-running container backends (ECS, Cloud Run, ACA).
- **Reverse mode** — Dials out to a backend-provided callback URL. Used by FaaS backends (Lambda, Cloud Functions, Azure Functions) where the container cannot accept inbound connections.

In both modes the agent supports:

- Executing arbitrary commands (`exec`) with optional TTY allocation
- Attaching to a long-running main process (`attach`) with output replay
- Streaming stdin/stdout/stderr over WebSocket with base64 encoding
- Sending signals (SIGTERM, SIGKILL, SIGINT, etc.) and terminal resizes
- Periodic health checks with configurable interval, timeout, and retries
- Bearer token authentication
- Zombie process reaping (PID 1 behavior)

## Project structure

```
agent/
├── cmd/sockerless-agent/   CLI entrypoint
│   └── main.go
├── server.go               Server, Config, ListenAndServe, ReverseConnect
├── message.go              Message type and constants
├── router.go               Incoming message dispatcher
├── session.go              Session interface and registry
├── exec.go                 ExecSession — fork+exec with PTY support
├── attach.go               AttachSession — attach to main process
├── process.go              MainProcess, RingBuffer (keep-alive mode)
├── healthcheck.go          HealthChecker (periodic health probes)
├── auth.go                 Bearer token middleware
├── wsclient.go             AgentConn — forward-mode WebSocket client
├── reverse.go              ReverseAgentConn — reverse-mode multiplexed client
└── reverse_test.go         Tests for reverse connection bridging
```

## Building

Build the agent binary:

```sh
cd agent
go build -o sockerless-agent ./cmd/sockerless-agent
```

## Usage

### Server mode

```sh
# Simple exec-only server on port 9111
sockerless-agent

# Keep-alive mode: run a long-lived process and allow attach/exec
sockerless-agent -keep-alive -- node server.js

# Custom address and log level
sockerless-agent -addr :8080 -log-level debug
```

### Reverse (FaaS) mode

```sh
sockerless-agent -callback http://backend:8080/internal/v1/agent/connect?id=abc&token=xyz
```

The agent dials the callback URL, upgrades to WebSocket, and handles messages from the backend. It reconnects with exponential backoff on disconnect (up to 10 retries).

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:9111` | Listen address (server mode) |
| `-callback` | | Reverse connect URL (enables reverse mode) |
| `-keep-alive` | `false` | Run remaining args as a main process |
| `-log-level` | `info` | Log level: debug, info, warn, error |

### Environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_AGENT_TOKEN` | Bearer token for authenticating WebSocket connections |

## WebSocket protocol

All messages are JSON. Stdout/stderr/stdin data is base64-encoded.

### Client to agent

| Type | Fields | Description |
|------|--------|-------------|
| `exec` | `id`, `cmd`, `env`, `workdir`, `tty` | Start a new exec session |
| `attach` | `id` | Attach to the main process |
| `stdin` | `id`, `data` | Send stdin data (base64) |
| `close_stdin` | `id` | Close stdin for a session |
| `signal` | `id`, `signal` | Send signal (e.g. `SIGTERM`) |
| `resize` | `id`, `width`, `height` | Resize terminal |

### Agent to client

| Type | Fields | Description |
|------|--------|-------------|
| `stdout` | `id`, `data` | Stdout chunk (base64) |
| `stderr` | `id`, `data` | Stderr chunk (base64) |
| `exit` | `id`, `code` | Session exited with code |
| `error` | `id`, `message` | Error message |

## Go library usage

The module also exposes a client API for connecting to a running agent from Go code.

### Forward connection

```go
conn, err := agent.Dial("localhost:9111", "my-token")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Bridge a Docker API exec request to the agent
exitCode := conn.BridgeExec(tcpConn, sessionID, cmd, env, workdir, tty)
```

### Reverse connection

```go
rc := agent.NewReverseAgentConn(wsConn)
defer rc.Close()

exitCode := rc.BridgeExec(tcpConn, sessionID, cmd, env, workdir, tty)
```

Both `BridgeExec` and `BridgeAttach` speak Docker's multiplexed stream protocol (8-byte header framing for non-TTY, raw passthrough for TTY).

## Testing

```sh
cd agent
go test -v ./...
```

The test suite covers reverse connection bridging, Docker mux protocol framing, TTY mode, concurrent session multiplexing, and connection lifecycle.

## Endpoints

When running in server mode the agent exposes:

- `GET /health` — Returns JSON with status, main process PID/exit info, and health check results
- `GET /ws` — WebSocket upgrade endpoint (requires `Authorization: Bearer <token>` header)
