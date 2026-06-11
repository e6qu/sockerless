# @sockerless/ui-bleephub

Standalone hub UI (separate product surface). GitHub-style shell (`src/components/Shell.tsx`) with pages in `src/pages/`, routed in `src/App.tsx` (React Router 7).

## Pages

When not logged in, every route redirects to `/ui/login`. Once authenticated:

- `/ui/` — overview
- `/ui/workflows` — workflow runs; `/ui/workflows/:id` for detail
- `/ui/runners` — registered runners
- `/ui/repos` — repositories; `/ui/repos/:owner/:repo` for detail
- `/ui/repos/:owner/:repo/issues` and `.../issues/:number`
- `/ui/repos/:owner/:repo/pulls` and `.../pulls/:number`
- `/ui/apps` — apps
- `/ui/oauth` — OAuth
- `/ui/metrics` — metrics

## Embedding

`make embed` in `bleephub/` copies this package's `dist/` to `bleephub/dist/` (see `make/go-app.mk`), which the binary bundles via `//go:embed all:dist` (`bleephub/ui_embed.go`) and serves at `/ui/` (default `:5555`, `make stack-bleephub-up`). A `-tags noui` build skips it.

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/internal`, `/health`, `/api`, and `/login` to a running bleephub on `:5555`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test` — vitest run (page tests in `src/__tests__/`).
- `bun run test:e2e` — Playwright tests.
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
