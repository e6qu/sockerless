import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  AppShell,
  ErrorBoundary,
  NavLinkButton,
  ToastProvider,
  type NavItem,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "./api.js";
import { DashboardPage } from "./pages/DashboardPage.js";
import { ComponentsPage } from "./pages/ComponentsPage.js";
import { ComponentDetailPage } from "./pages/ComponentDetailPage.js";
import { ContainersPage } from "./pages/ContainersPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";
import { ContextsPage } from "./pages/ContextsPage.js";
import { ProcessesPage } from "./pages/ProcessesPage.js";
import { ProcessDetailPage } from "./pages/ProcessDetailPage.js";
import { CleanupPage } from "./pages/CleanupPage.js";
import { TopologyPage } from "./pages/TopologyPage.js";
import { InstanceLogsPage } from "./pages/InstanceLogsPage.js";
import { ProjectConsolePage } from "./pages/ProjectConsolePage.js";
import { TopologyResourcesPage } from "./pages/TopologyResourcesPage.js";

const navItems: NavItem[] = [
  { label: "Dashboard", to: "/ui/" },
  { label: "Topology", to: "/ui/topology" },
  { label: "Components", to: "/ui/components" },
  { label: "Processes", to: "/ui/processes" },
  { label: "Containers", to: "/ui/containers" },
  { label: "Cleanup", to: "/ui/cleanup" },
  { label: "Metrics", to: "/ui/metrics" },
  { label: "Contexts", to: "/ui/contexts" },
];

const api = new AdminApiClient();

function renderNavLink(item: NavItem) {
  return (
    <NavLink to={item.to} end={item.to === "/ui/"}>
      {({ isActive }) => <NavLinkButton active={isActive}>{item.label}</NavLinkButton>}
    </NavLink>
  );
}

export function App() {
  const { data: session } = useQuery({
    queryKey: ["auth-session"],
    queryFn: () => api.session(),
    retry: false,
  });
  const accountControl = session?.authenticated ? (
    <div className="flex items-center gap-2" aria-label="Signed-in user" data-shauth-user={session.name}>
      <span
        className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
        aria-hidden="true"
        style={{ background: "var(--color-accent-soft)", color: "var(--color-fg)" }}
      >
        {(session.name || "?").slice(0, 1).toUpperCase()}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium" style={{ color: "var(--color-fg)" }}>{session.name}</p>
        <p className="text-[10px] uppercase tracking-[0.12em]" style={{ color: "var(--color-fg-subtle)" }}>{session.role}</p>
      </div>
      <form method="post" action="/auth/logout">
        <button className="text-xs underline" type="submit" aria-label="Sign out" data-shauth-sign-out>Sign out</button>
      </form>
    </div>
  ) : undefined;
  return (
    <ErrorBoundary>
      <ToastProvider>
        <BrowserRouter>
        <AppShell
          kicker="sockerless · operator"
          title="Admin"
          navItems={navItems}
          renderLink={renderNavLink}
          accountControl={accountControl}
        >
          <Routes>
            <Route path="/ui/" element={<DashboardPage />} />
            <Route path="/ui/components" element={<ComponentsPage />} />
            <Route
              path="/ui/components/:name"
              element={<ComponentDetailPage />}
            />
            <Route path="/ui/topology" element={<TopologyPage />} />
            <Route
              path="/ui/topology/resources"
              element={<TopologyResourcesPage />}
            />
            <Route
              path="/ui/topology/:project/:instance/logs"
              element={<InstanceLogsPage />}
            />
            <Route
              path="/ui/topology/:project/console"
              element={<ProjectConsolePage />}
            />
            <Route path="/ui/processes" element={<ProcessesPage />} />
            <Route path="/ui/processes/:name" element={<ProcessDetailPage />} />
            <Route path="/ui/containers" element={<ContainersPage />} />
            <Route path="/ui/cleanup" element={<CleanupPage />} />
            <Route path="/ui/metrics" element={<MetricsPage />} />
            <Route path="/ui/contexts" element={<ContextsPage />} />
            <Route
              path="*"
              element={
                <div
                  className="p-8 font-mono uppercase tracking-[0.18em]"
                  style={{ color: "var(--color-fg-subtle)", fontSize: "0.78rem" }}
                >
                  — page not found —
                </div>
              }
            />
          </Routes>
        </AppShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
