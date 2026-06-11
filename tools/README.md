# Tools

Standalone Go utilities (each its own module, built with `GOWORK=off`). Neither ships in any binary; both are developer/CI aids.

## check-backend-coverage

Verifies that every backend explicitly implements all methods of the `api.Backend` interface rather than silently inheriting `BaseServer`'s in-memory defaults via embedding — the compile-time `var _ api.Backend = (*Server)(nil)` check alone can't catch that, because the embedded `*core.BaseServer` satisfies the interface. The backend list (core, docker, ecs, cloudrun, aca, lambda, cloudrun-functions, azure-functions) is declared in `check-backend-coverage/main.go`.

```sh
go run . [--enforce]   # report missing methods; --enforce exits 1 if any
go run . --generate    # generate backend_delegates_gen.go per backend
```

Invocations:

- `make check-backend-coverage` / `make check-backend-coverage-enforce` (repo-root Makefile).
- CI runs the `--enforce` form in the `build-gates` job of [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) on every PR.

Related spec: [`specs/API_SURFACE.md`](../specs/API_SURFACE.md) (the `api.Backend` method table), [`specs/BACKENDS.md`](../specs/BACKENDS.md) (the self-dispatch pattern).

## http-trace

A diagnostic HTTP proxy that logs every request and response flowing through it, including hijacked connections (attach / exec) — on `101 Switching Protocols` it switches to raw TCP bridging. Point the Docker CLI at it to capture the exact wire sequence between a Docker client and a sockerless backend:

```sh
go run ./tools/http-trace -listen :13375 -upstream http://127.0.0.1:3375
DOCKER_HOST=tcp://127.0.0.1:13375 docker run -d alpine echo hi
```

Manual-use only; not wired into any Makefile target or CI job.
