# @sockerless/ui-simulator-gcp

GCP Simulator dashboard. A `<SimulatorApp title="GCP Simulator" />` shell from `@sockerless/ui-core` with per-service pages under `src/pages/`, routed in `src/main.tsx` (React Router 7).

## Pages

- `/ui/` — overview
- `/ui/cloudrun` — Cloud Run jobs
- `/ui/functions` — Cloud Functions
- `/ui/ar` — Artifact Registry
- `/ui/gcs` — GCS buckets
- `/ui/logging` — Cloud Logging

Pages fetch the simulator's `/sim/*` UI endpoints via `src/api.ts`; the shell polls `/health` through the core simulator hooks.

## Embedding

`make embed` in `simulators/gcp/` copies this package's `dist/` to `simulators/gcp/dist/` (see `make/go-app.mk`), which the binary bundles via `//go:embed all:dist` (`simulators/gcp/ui_embed.go`) and serves at `/ui/`. A `-tags noui` build skips it.

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/health` and `/sim` to a running simulator on `:4567`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test:e2e` — Playwright tests.
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
