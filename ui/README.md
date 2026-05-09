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

- `bun install` from this directory.
- `bunx turbo run build` builds every package; outputs go to each backend's `dist/` for embed by the Go binary (`go:embed all:dist`).
- `bunx turbo run test` runs vitest across every package.
- `bunx turbo run typecheck` runs `tsc --noEmit` across every package.

## Per-app quick start

Each backend app is a Vite project under `packages/backend-<name>/`. To work on one in isolation:

```sh
cd ui/packages/backend-ecs
bun run dev   # vite dev server, hot reload, hits the live backend API at localhost:3375
```

The Go backend serves the built UI at `/ui/` via `go:embed`.
