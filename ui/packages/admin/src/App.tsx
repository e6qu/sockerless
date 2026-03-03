import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { AppShell, type NavItem } from "@sockerless/ui-core/components";
import { ErrorBoundary } from "@sockerless/ui-core/components";
import { DashboardPage } from "./pages/DashboardPage.js";
import { ComponentsPage } from "./pages/ComponentsPage.js";
import { ComponentDetailPage } from "./pages/ComponentDetailPage.js";
import { ContainersPage } from "./pages/ContainersPage.js";
import { ResourcesPage } from "./pages/ResourcesPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";
import { ContextsPage } from "./pages/ContextsPage.js";
import { ProcessesPage } from "./pages/ProcessesPage.js";
import { ProcessDetailPage } from "./pages/ProcessDetailPage.js";
import { CleanupPage } from "./pages/CleanupPage.js";
import { ProjectsPage } from "./pages/ProjectsPage.js";
import { ProjectCreatePage } from "./pages/ProjectCreatePage.js";
import { ProjectDetailPage } from "./pages/ProjectDetailPage.js";
import { ProjectLogsPage } from "./pages/ProjectLogsPage.js";

const navItems: NavItem[] = [
  { label: "Dashboard", to: "/ui/" },
  { label: "Projects", to: "/ui/projects" },
  { label: "Components", to: "/ui/components" },
  { label: "Processes", to: "/ui/processes" },
  { label: "Containers", to: "/ui/containers" },
  { label: "Resources", to: "/ui/resources" },
  { label: "Cleanup", to: "/ui/cleanup" },
  { label: "Metrics", to: "/ui/metrics" },
  { label: "Contexts", to: "/ui/contexts" },
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
        <AppShell title="Sockerless Admin" navItems={navItems} renderLink={renderNavLink}>
          <Routes>
            <Route path="/ui/" element={<DashboardPage />} />
            <Route path="/ui/components" element={<ComponentsPage />} />
            <Route path="/ui/components/:name" element={<ComponentDetailPage />} />
            <Route path="/ui/projects" element={<ProjectsPage />} />
            <Route path="/ui/projects/new" element={<ProjectCreatePage />} />
            <Route path="/ui/projects/:name" element={<ProjectDetailPage />} />
            <Route path="/ui/projects/:name/logs" element={<ProjectLogsPage />} />
            <Route path="/ui/processes" element={<ProcessesPage />} />
            <Route path="/ui/processes/:name" element={<ProcessDetailPage />} />
            <Route path="/ui/containers" element={<ContainersPage />} />
            <Route path="/ui/resources" element={<ResourcesPage />} />
            <Route path="/ui/cleanup" element={<CleanupPage />} />
            <Route path="/ui/metrics" element={<MetricsPage />} />
            <Route path="/ui/contexts" element={<ContextsPage />} />
            <Route path="*" element={<div className="p-4 text-sm text-gray-500 dark:text-gray-400">Page not found</div>} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
