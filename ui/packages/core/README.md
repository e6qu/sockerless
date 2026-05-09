# @sockerless/ui-core

Shared design system, API client, hooks, and shells for every Sockerless UI package.

## Exports

### Components
- **Layout / shell** — `AppShell`, `NavLinkButton`, `BackendApp`, `SimulatorApp`, `PageHeading`, `BackendInfoCard`.
- **Data** — `DataTable` (TanStack Table; row-click → detail modal pattern), `MetricsCard`, `StatusBadge`, `LogViewer`.
- **Feedback** — `ErrorBoundary` (catastrophic), `Toast` + `ToastProvider` + `useToast` + `useReportError` + `useToastQueryErrors` (transient), `InlineError` (in-page).
- **Interaction** — `Button`, `RefreshButton`, `Modal`, `ContainerDetailModal`, `Spinner`.
- **Theming** — `ThemeToggle` (lives in `AppShell`'s footer; flips the `dark` class via `useTheme`).

### Hooks
- **API** — `ApiClientProvider`, `useApiClient`, `useHealth`, `useStatus`, `useContainers`, `useMetrics`, `useResources`, `useCheck`, `useInfo` (TanStack Query wrappers; auto-poll 5–10s).
- **Simulator** — `useSimHealth`, `useSimSummary`.
- **Theme** — `useTheme` (localStorage + prefers-color-scheme + dark default).

### Styles
- `src/styles/tokens.css` — design tokens consumed by every app's `index.css` after `@import "tailwindcss";`.

## Development

- `bun run dev` — no dev server (this is a library); use a backend's app instead.
- `bun run test` — vitest run.
- `bun run typecheck` — `tsc --noEmit`.

## Test environment notes

- `vitest.config.ts` pins jsdom to `url: "http://localhost/"` so localStorage works (jsdom gates webstorage on origin).
- `src/test-setup.ts` polyfills bun's broken `localStorage` (object without methods) and missing `matchMedia`, plus wires `@testing-library/react`'s `cleanup()` between tests.
