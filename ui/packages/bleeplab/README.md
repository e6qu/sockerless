# @sockerless/ui-bleeplab

Dashboard UI for [bleeplab](../../../bleeplab/README.md), the GitLab control-plane simulator. A GitLab-style shell with pages in `src/pages/`, routed in `src/App.tsx` (React Router 7). It is read-only: it reads bleeplab's `/internal/*` projections and public `/api/v4` surface to show projects, pipelines, jobs, and registered runners.

## Pages

- `/ui/` — overview
- `/ui/projects` — projects; `/ui/projects/:id` for detail
- `/ui/pipelines` — pipelines; `/ui/pipelines/:id` for detail
- `/ui/jobs/:id` — job detail (status, trace, artifacts)
- `/ui/runners` — registered runners

## Embedding

`make embed` in `bleeplab/` copies this package's `dist/` to `bleeplab/dist/` (see `make/go-app.mk`), which the binary bundles via `//go:embed all:dist` (`bleeplab/ui_embed.go`) and serves at `/ui/` (default `:8929`). A `-tags noui` build skips it (`bleeplab/ui_noembed.go`).

## Development

- `bun run dev` — Vite dev server (`:5173`), proxying `/internal`, `/health`, and `/api` to a running bleeplab on `:8929`.
- `bun run build` — production bundle into `dist/`.
- `bun run preview` — serve the built bundle.
- `bun run test` — vitest run (page tests in `src/__tests__/`).
- `bun run typecheck` — `tsc --noEmit`.

The package `Makefile` wraps these as `make build` / `run` / `preview` / `test` / `lint` / `clean` (see `make/ui-app.mk`).

## See also

- [`bleeplab/README.md`](../../../bleeplab/README.md) — the server this UI dashboards.
- [Workspace README](../../README.md) — dev-stack targets, ports, design system, error UX.
- [`@sockerless/ui-core`](../core/README.md) — shared components, hooks, tokens.
