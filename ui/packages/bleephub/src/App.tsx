import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { AppShell, type NavItem } from "@sockerless/ui-core/components";
import { ErrorBoundary } from "@sockerless/ui-core/components";
import { OverviewPage } from "./pages/OverviewPage.js";
import { WorkflowsPage } from "./pages/WorkflowsPage.js";
import { WorkflowDetailPage } from "./pages/WorkflowDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";
import { ReposPage } from "./pages/ReposPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";

const navItems: NavItem[] = [
  { label: "Overview", to: "/ui/" },
  { label: "Workflows", to: "/ui/workflows" },
  { label: "Runners", to: "/ui/runners" },
  { label: "Repos", to: "/ui/repos" },
  { label: "Metrics", to: "/ui/metrics" },
];

function renderNavLink(item: NavItem) {
  return (
    <NavLink
      to={item.to}
      end={item.to === "/ui/"}
      className={({ isActive }) =>
        `block rounded-md px-3 py-2 text-sm font-medium ${
          isActive
            ? "bg-blue-50 text-blue-700 dark:bg-blue-900 dark:text-blue-200"
            : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
        }`
      }
    >
      {item.label}
    </NavLink>
  );
}

export function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AppShell title="bleephub" navItems={navItems} renderLink={renderNavLink}>
          <Routes>
            <Route path="/ui/" element={<OverviewPage />} />
            <Route path="/ui/workflows" element={<WorkflowsPage />} />
            <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
            <Route path="/ui/runners" element={<RunnersPage />} />
            <Route path="/ui/repos" element={<ReposPage />} />
            <Route path="/ui/metrics" element={<MetricsPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
