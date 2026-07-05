# @sockerless/ui-bleephub

Standalone hub UI (separate product surface) — a functional GitHub-faithful clone.
Pages live in `src/pages/` and are routed in `src/App.tsx` (React Router 7) inside
`src/components/Shell.tsx`, which renders the global header `src/components/AppHeader.tsx`
above the routed content.

## App chrome

`AppHeader` mirrors github.com's global header: a hamburger opening a global-nav drawer
(a "GitHub" section and a bleephub-server "Operations" section), the brand, a search box,
a "+" create menu, Issues / Pull-requests quick links, a notifications bell with an
unread badge, and an avatar dropdown (profile, your repos/gists/packages/codespaces,
settings, theme toggle, sign out). The server-operational surfaces have no github.com
equivalent, so they live in the drawer's Operations section and under `/ui/admin`, not the
GitHub-shaped nav.

## Pages

When not logged in, every route redirects to `/ui/login`. Once authenticated (routes from
`src/App.tsx`):

GitHub surfaces:
- `/ui/` — dashboard (top repositories + activity feed)
- `/ui/:login` and `/ui/users/:login` — user profile
- `/ui/repos` — repositories; `/ui/repos/:owner/:repo` — repo Code (About sidebar, file
  tree, README); plus `.../issues[/:number]`, `.../pulls[/:number]`, `.../discussions[/:number]`,
  `.../actions` and `.../actions/runs/:runId`, `.../insights`, `.../projects-classic`,
  `.../security/{secret-scanning,code-scanning,dependabot,advisories}`,
  `.../settings[/branch-protection|/secrets]`, `.../labels`, `.../milestones`,
  `.../stargazers`, `.../watchers`, `.../forks`, `.../deployments`, `.../hooks/:hookId/deliveries`
- `/ui/orgs/:org` — org overview; plus `.../repos`, `.../people`, `.../teams`, `.../packages`,
  `.../rulesets`, `.../governance`, `.../copilot`, `.../hooks[/:hookId/deliveries]`
- `/ui/gists`, `/ui/packages`, `/ui/codespaces`, `/ui/migrations`, `/ui/notifications`,
  `/ui/search`, `/ui/account`

Operations (`/ui/admin*`) — bleephub-server surfaces with no github.com equivalent:
- `/ui/admin` — system-status console; `/ui/admin/{users,orgs,teams,enterprise,audit-log,storage}`
- `/ui/workflows[/:id]` — cross-repo workflow runs; `/ui/runners`; `/ui/metrics`;
  `/ui/apps` (GitHub Apps); `/ui/oauth` (OAuth Apps)

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
