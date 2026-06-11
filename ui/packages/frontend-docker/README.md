# @sockerless/ui-frontend-docker

Docker-frontend UI: an observability view for the docker proxy. Single screen, no router — `src/App.tsx` renders one Overview inside the core `AppShell`.

## What it renders

- Proxy status: `docker_addr → backend_addr` and uptime.
- Metrics cards: Docker requests, goroutines, heap, uptime.

Data comes from `/healthz`, `/status`, and `/metrics` via `src/api.ts` (TanStack Query, 5–10s auto-poll).

## Embedding

None — unlike the other UI apps, no Go module embeds this package's `dist/`; the root `Makefile` lists it as standalone.

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/healthz`, `/status`, and `/metrics` to a running docker frontend on `:9200`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test:e2e` — Playwright tests.
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
