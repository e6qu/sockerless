import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import {
  AppShell,
  ErrorBoundary,
  NavLinkButton,
  ToastProvider,
  type NavItem,
} from "@sockerless/ui-core/components";
import { OverviewPage } from "./pages/OverviewPage.js";
import { WorkflowsPage } from "./pages/WorkflowsPage.js";
import { WorkflowDetailPage } from "./pages/WorkflowDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";
import { ReposPage } from "./pages/ReposPage.js";
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

export function App() {
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
            <Routes>
              <Route path="/ui/" element={<OverviewPage />} />
              <Route path="/ui/workflows" element={<WorkflowsPage />} />
              <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
              <Route path="/ui/runners" element={<RunnersPage />} />
              <Route path="/ui/repos" element={<ReposPage />} />
              <Route path="/ui/apps" element={<AppsPage />} />
              <Route path="/ui/oauth" element={<OAuthPage />} />
              <Route path="/ui/metrics" element={<MetricsPage />} />
            </Routes>
          </AppShell>
        </BrowserRouter>
      </ToastProvider>
    </ErrorBoundary>
  );
}
