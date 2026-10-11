# Tools

Standalone Go utilities (each its own module, built with `GOWORK=off`). None ships in any binary; all are developer/CI aids.

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

## check-image-pins

Keeps every image a Dockerfile, workflow, deployment compose file, `.gitlab-ci.yml`, shell script or Makefile pulls off Docker Hub and current. Docker Hub limits anonymous pulls per address and the shared CI runners exhaust that limit, so each image is a Docker Hub copy pinned by tag and digest: `public.ecr.aws/docker/library/<name>` for official images and `mirror.gcr.io/<repository>` for the rest. Go sources are not scanned: the backends name public.ecr.aws in the code that rewrites workload images, and their tests feed the reference parsers made-up digests.

```sh
go run .              # refuse Docker Hub images and copies without a tag and digest (no network)
go run . --freshness  # also compare every pin with Docker Hub and its mirror
```

`--freshness` reads Docker Hub's tag API and the mirror's manifest for each pin and spaces its ECR Public requests 1.1 seconds apart, under that registry's one-request-per-second anonymous limit:

- **ok**: the pin is the digest Docker Hub serves for its tag, and the mirror serves the same digest.
- **HELD**: Docker Hub moved the tag, or published a newer release with the same variant and major version, less than 24 hours ago.
- **DRIFT** (fails): that change is at least 24 hours old, or the mirror serves another digest than the pin.

A release Docker Hub has not pushed for 90 days is out of support and does not count as newer (Ubuntu's interim releases, for example). Go stays on the 1.26 line on purpose: a newer Go release is reported, not failed, until CI's golangci-lint can load it.

To bump a pin, take the digest Docker Hub serves for the tag and confirm the mirror serves the same one, for example with `docker buildx imagetools inspect public.ecr.aws/docker/library/<name>:<tag>`.

Invocations:

- `make check-image-pins` / `make check-image-pins-freshness`.
- CI runs the offline form in the `build-gates` job of [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), and pre-commit runs it on every commit.
- [`.github/workflows/image-pins.yml`](../.github/workflows/image-pins.yml) runs `--freshness` on pull requests, on pushes to `main` and daily.

## http-trace

A diagnostic HTTP proxy that logs every request and response flowing through it, including hijacked connections (attach / exec) — on `101 Switching Protocols` it switches to raw TCP bridging. Point the Docker CLI at it to capture the exact wire sequence between a Docker client and a sockerless backend:

```sh
go run ./tools/http-trace -listen :13375 -upstream http://127.0.0.1:3375
DOCKER_HOST=tcp://127.0.0.1:13375 docker run -d alpine echo hi
```

Manual-use only; not wired into any Makefile target or CI job.
