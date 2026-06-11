# @sockerless/ui-backend-lambda

Lambda Backend UI. The entire app is `<BackendApp title="Lambda Backend" />` from `@sockerless/ui-core`, plus an `index.html` that loads the design-system fonts and this app's `index.css`.

## Pages

All routes come from the shared `BackendApp` shell (`core/src/components/BackendApp.tsx`):

- `/ui/` — overview
- `/ui/containers` — containers table (row click opens `ContainerDetailModal`)
- `/ui/resources` — resource registry
- `/ui/metrics` — runtime metrics

Data comes from the backend's `/internal/v1/*` endpoints through the core `ApiClient` (TanStack Query, 5–10s auto-poll).

## Embedding

`make embed` in `backends/lambda/` copies this package's `dist/` to `backends/lambda/dist/` (see `make/go-app.mk`), which the binary bundles via `//go:embed all:dist` (`backends/lambda/ui_embed.go`) and serves at `/ui/`. A `-tags noui` build skips it.

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/internal` to a running backend on `:3375`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test:e2e` — Playwright tests.
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
