# Sockerless UI

Bun + Turborepo workspace. 12 packages share a single design system + API client; per-backend / per-simulator apps are thin (a couple of dozen lines apiece).

## Packages

| Package | Role |
|---|---|
| `core` | Design system (tokens, components), API client, TanStack Query hooks, `BackendApp` + `SimulatorApp` shells. Imported by every other UI package. |
| `backend-{ecs,lambda,cloudrun,gcf,aca,azf,docker}` | Per-backend Vite apps. Each is `<BackendApp title="..." />` plus an `index.html` that loads the design-system fonts + index.css. |
| `frontend-docker` | Docker-frontend UI (in-process docker daemon view). |
| `simulator-{aws,gcp,azure}` | Per-cloud simulator dashboards (sim-only routes). |
| `admin` | Cross-backend admin dashboard (lists every running backend, drills into each). |
| `bleephub` | Standalone hub UI (separate product surface). |

## Design system

`core/src/styles/tokens.css` defines:
- Surfaces (warm zinc, never blue-slate) + dark mode via the `.dark` class.
- Per-app accent colour (each app overrides `--color-accent` in its own `index.css`).
- Two voices: serif display (Fraunces) + monospace body (JetBrains Mono).
- Sharp corners, minimal motion, subtle dotted-grid background.

The `dark` class is toggled by the `<ThemeToggle>` component (sidebar footer) backed by the `useTheme` hook (localStorage + prefers-color-scheme + dark default).

## Error UX

- `<ErrorBoundary>` (top of every app) catches unhandled exceptions.
- `<ToastProvider>` (just inside ErrorBoundary) renders a top-right notification stack. Push via `useToast().push({ tone, title, body })`; helper `useReportError()` formats unknown errors as toasts. `useToastQueryErrors(error)` wraps a TanStack Query error.
- `<InlineError title detail action>` — in-page banner for operation failures (data load failed, mutation rejected). Rendered alongside a "Retry" button on every page that loads remote data.

## Modals

`<Modal>` wraps the native `<dialog>` element so focus trapping + ESC + backdrop click come from the platform. `<ContainerDetailModal>` is the canonical example — opened from a row click on the Containers page.

## Auto-refresh

TanStack Query polls each endpoint at a fixed interval (5–10s). `refetchIntervalInBackground` is the TanStack default (false) so polling pauses when the tab is hidden.

## Build / test

From this directory (`ui/`):

- `bun install` — install workspace dependencies.
- `bunx turbo run build` — build every package; outputs go to each backend's `dist/` for embed by the Go binary (`go:embed all:dist`).
- `bunx turbo run test` — run vitest across every package.
- `bunx turbo run typecheck` — `tsc --noEmit` across every package.

## Starting and stopping a full dev stack (recommended)

The repo's root Makefile bundles the simulator + backend + admin UI as a single stack. From the **repo root** (one directory above this one):

```sh
make stack-aws-ecs        # sim-aws + backend-ecs + admin
make stack-aws-lambda     # sim-aws + backend-lambda + admin
make stack-gcp-cloudrun   # sim-gcp + backend-cloudrun + admin
make stack-gcp-gcf        # sim-gcp + backend-gcf + admin
make stack-azure-aca      # sim-azure + backend-aca + admin
make stack-azure-azf      # sim-azure + backend-azf + admin

make stack-status         # show which components are running + their PIDs
make stack-down           # stop every running stack process and clean up .stack-pids/
```

Each `stack-*-*` target builds the three components, starts them as background processes, and writes their PIDs to `.stack-pids/` so `stack-down` can find them later. Internally these targets compose `make start-component` calls (see below + `docs/ADMIN_ORCHESTRATION.md`). Logs land in `.stack-pids/sim.log`, `.stack-pids/backend.log`, `.stack-pids/admin.log`.

Default ports (overridable via env in `make/stack.mk`):
- AWS sim `:4566`, GCP sim `:4567`, Azure sim `:4568`
- Backend `:3375`
- Admin UI `:9090`
- bleephub `:5555` (separate target: `make stack-bleephub-up`)

Open the admin UI at `http://localhost:9090/ui/` once `stack-status` reports it green.

### Arbitrary topology (multiple sims / backends / bleephubs)

For richer topologies, use the granular per-component targets — admin's REST surface drives these too:

```sh
make start-component KIND=sim     CLOUD=aws  NAME=sim-1     PORT=4566
make start-component KIND=sim     CLOUD=gcp  NAME=sim-2     PORT=4567
make start-component KIND=backend CLOUD=aws  BACKEND=ecs    NAME=ecs-1 PORT=3375 SIM_PORT=4566
make start-component KIND=bleephub             NAME=bleep-1   PORT=5555

make logs-component  NAME=ecs-1 LINES=200
make stop-component  NAME=ecs-1
make rebuild-component KIND=backend CLOUD=aws BACKEND=ecs

make status-components       # show every running component
make stop-components         # SIGTERM every running component
```

Persist the topology in `sockerless.yaml` at the repo root and admin will reconcile + drive lifecycle automatically. See `docs/ADMIN_ORCHESTRATION.md` for the full schema + REST surface.

## Working on a single UI package (vite hot reload)

When you only want to iterate on one backend's UI without booting the full stack, run that package's Vite dev server. It assumes the backend is already running on `:3375` (start it via `make stack-X-Y` from the root).

```sh
cd ui/packages/backend-ecs
bun run dev      # vite serves on :5173 by default; proxies API calls to :3375
# Ctrl-C to stop the dev server.
```

The Go backend serves the *built* UI at `/ui/` via `go:embed`; the dev server is only for hot-reload-during-development.

## Just the UI build (no backend)

```sh
cd ui
bun install
bunx turbo run build           # all packages
# or one package: bun --filter '@sockerless/ui-admin' run build
```
