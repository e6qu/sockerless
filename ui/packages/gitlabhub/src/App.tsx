import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { AppShell, type NavItem } from "@sockerless/ui-core/components";
import { ErrorBoundary } from "@sockerless/ui-core/components";
import { OverviewPage } from "./pages/OverviewPage.js";
import { PipelinesPage } from "./pages/PipelinesPage.js";
import { PipelineDetailPage } from "./pages/PipelineDetailPage.js";
import { RunnersPage } from "./pages/RunnersPage.js";
import { ProjectsPage } from "./pages/ProjectsPage.js";
import { MetricsPage } from "./pages/MetricsPage.js";

const navItems: NavItem[] = [
  { label: "Overview", to: "/ui/" },
  { label: "Pipelines", to: "/ui/pipelines" },
  { label: "Runners", to: "/ui/runners" },
  { label: "Projects", to: "/ui/projects" },
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
        <AppShell title="gitlabhub" navItems={navItems} renderLink={renderNavLink}>
          <Routes>
            <Route path="/ui/" element={<OverviewPage />} />
            <Route path="/ui/pipelines" element={<PipelinesPage />} />
            <Route path="/ui/pipelines/:id" element={<PipelineDetailPage />} />
            <Route path="/ui/runners" element={<RunnersPage />} />
            <Route path="/ui/projects" element={<ProjectsPage />} />
            <Route path="/ui/metrics" element={<MetricsPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
