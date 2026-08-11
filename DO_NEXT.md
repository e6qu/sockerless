# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next

The simulators were extracted to
[sockerless-cloud](https://github.com/e6qu/sockerless-cloud); this repository
consumes them as pinned Go modules (`tests/go.mod` `tool` directives,
`make install-simulators`, Docker `ARG SOCKERLESS_CLOUD_VERSION`). The
simulator-side burn-downs that previously lived here (IAM resource-derivation
BUG-2909, spec-pattern allowlists, service-slice ratchets) continue in that
repository's DO_NEXT.md.

In this repository:

- Watch the first post-extraction CI run; the harness paths changed shape
  (sims build from the module cache instead of in-tree source, smoke images
  `go install` at the pinned version) and hosted runners are the first
  environment to exercise every Docker path end-to-end.
- When bumping the simulator version, bump every pin in one PR: `tests/go.mod`
  (`make -C tests upgrade-deps` covers it), the `ARG SOCKERLESS_CLOUD_VERSION`
  defaults across smoke/upstream/e2e Dockerfiles, the `context:` tag in
  `deploy/compose.build.yaml`, and the pinned ref in
  `.github/workflows/live-tests-lambda.yml`.
- Branch protection still lists the removed simulator CI checks (sim, tf,
  simulators-quality, firecracker, browser sim packages) as required; they
  must be unrequired before this branch's PR can merge green.
- BUG-2922 (Docker Engine advisories → moby/moby client migration) remains
  the largest open local item, now scoped here to the Docker backend.
