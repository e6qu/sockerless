import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { AppShell, type NavItem } from "./AppShell.js";
import { ErrorBoundary } from "./ErrorBoundary.js";
import { OverviewPage } from "../pages/OverviewPage.js";
import { ContainersPage } from "../pages/ContainersPage.js";
import { ResourcesPage } from "../pages/ResourcesPage.js";
import { MetricsPage } from "../pages/MetricsPage.js";

const navItems: NavItem[] = [
  { label: "Overview", to: "/ui/" },
  { label: "Containers", to: "/ui/containers" },
  { label: "Resources", to: "/ui/resources" },
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

export interface BackendAppProps {
  title: string;
}

export function BackendApp({ title }: BackendAppProps) {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AppShell title={title} navItems={navItems} renderLink={renderNavLink}>
          <Routes>
            <Route path="/ui/" element={<OverviewPage />} />
            <Route path="/ui/containers" element={<ContainersPage />} />
            <Route path="/ui/resources" element={<ResourcesPage />} />
            <Route path="/ui/metrics" element={<MetricsPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
