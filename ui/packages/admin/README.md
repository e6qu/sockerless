# @sockerless/ui-admin

Cross-backend operator dashboard: component health, topology lifecycle, processes, containers, cleanup, metrics, and CLI contexts. Pages live in `src/pages/`, routed in `src/App.tsx` (React Router 7).

## Pages

- `/ui/` — dashboard
- `/ui/topology` — topology lifecycle; plus `/ui/topology/resources`, `/ui/topology/:project/:instance/logs`, and `/ui/topology/:project/console`
- `/ui/components` — registered components; `/ui/components/:name` for detail
- `/ui/processes` — process lifecycle; `/ui/processes/:name` for detail
- `/ui/containers` — containers across backends
- `/ui/cleanup` — cleanup actions
- `/ui/metrics` — metrics
- `/ui/contexts` — CLI contexts

Page-level failures render through `src/components/ErrorPanel.tsx`, which surfaces concrete recovery commands (`make stack-status`, `make stack-down`, `make start-component ...`).

## Embedding

`make embed` in `cmd/sockerless-admin/` copies this package's `dist/` to `cmd/sockerless-admin/dist/` (see `make/go-app.mk`), which the binary bundles via `//go:embed all:dist` (`cmd/sockerless-admin/ui_embed.go`) and serves at `/ui/` (default `:9090`). A `-tags noui` build skips it.

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/api` to a running admin server on `:9090`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test` — vitest run (page tests in `src/__tests__/`).
- `bun run test:e2e` — Playwright tests.
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
