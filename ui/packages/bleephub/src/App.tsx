import { BrowserRouter, Routes, Route, NavLink, Navigate } from "react-router";
import {
  AppShell,
  ErrorBoundary,
  NavLinkButton,
  ToastProvider,
  type NavItem,
} from "@sockerless/ui-core/components";
import { isLoggedIn, clearToken } from "./api.js";
import { LoginPage } from "./pages/LoginPage.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { WorkflowsPage } from "./pages/WorkflowsPage.js";
import { WorkflowDetailPage } from "./pages/WorkflowDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";
import { ReposPage } from "./pages/ReposPage.js";
import { RepoDetailPage } from "./pages/RepoDetailPage.js";
import { IssuesPage } from "./pages/IssuesPage.js";
import { PullsPage } from "./pages/PullsPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";
import { AppsPage } from "./pages/AppsPage.js";
import { OAuthPage } from "./pages/OAuthPage.js";

const navItems: NavItem[] = [
  { label: "Overview", to: "/ui/" },
  { label: "Workflows", to: "/ui/workflows" },
  { label: "Runners", to: "/ui/runners" },
  { label: "Repos", to: "/ui/repos" },
  { label: "Apps", to: "/ui/apps" },
  { label: "OAuth", to: "/ui/oauth" },
  { label: "Metrics", to: "/ui/metrics" },
];

function renderNavLink(item: NavItem) {
  return (
    <NavLink to={item.to} end={item.to === "/ui/"}>
      {({ isActive }) => <NavLinkButton active={isActive}>{item.label}</NavLinkButton>}
    </NavLink>
  );
}

function AppNav() {
  return (
    <div
      className="flex items-center justify-end px-3 py-1"
      style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
    >
      <button
        onClick={() => {
          clearToken();
          window.location.href = "/ui/login";
        }}
        className="hover:underline"
      >
        Sign out
      </button>
    </div>
  );
}

export function App() {
  if (!isLoggedIn()) {
    return (
      <ErrorBoundary>
        <BrowserRouter>
          <Routes>
            <Route path="/ui/login" element={<LoginPage />} />
            <Route path="/ui/*" element={<Navigate to="/ui/login" replace />} />
          </Routes>
        </BrowserRouter>
      </ErrorBoundary>
    );
  }

  return (
    <ErrorBoundary>
      <ToastProvider>
        <BrowserRouter>
          <AppShell
            kicker="github · simulator"
            title="bleephub"
            navItems={navItems}
            renderLink={renderNavLink}
          >
            <AppNav />
            <Routes>
              <Route path="/ui/" element={<OverviewPage />} />
              <Route path="/ui/workflows" element={<WorkflowsPage />} />
              <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
              <Route path="/ui/runners" element={<RunnersPage />} />
              <Route path="/ui/repos" element={<ReposPage />} />
              <Route path="/ui/repos/:owner/:repo" element={<RepoDetailPage />} />
              <Route path="/ui/repos/:owner/:repo/issues" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/issues/:number" element={<IssuesPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls" element={<PullsPage />} />
              <Route path="/ui/repos/:owner/:repo/pulls/:number" element={<PullsPage />} />
              <Route path="/ui/apps" element={<AppsPage />} />
              <Route path="/ui/oauth" element={<OAuthPage />} />
              <Route path="/ui/metrics" element={<MetricsPage />} />
              {/* A logged-in user hitting /ui/login (bookmark) or any
                  unknown /ui/* path lands back on the dashboard. */}
              <Route path="/ui/login" element={<Navigate to="/ui/" replace />} />
              <Route path="/ui/*" element={<Navigate to="/ui/" replace />} />
            </Routes>
          </AppShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
