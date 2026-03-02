import { type ReactNode } from "react";
import { BrowserRouter, Routes, Route, NavLink } from "react-router";
import { AppShell, type NavItem } from "./AppShell.js";
import { ErrorBoundary } from "./ErrorBoundary.js";

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

export interface SimulatorAppProps {
  title: string;
  navItems: NavItem[];
  children: ReactNode;
}

export function SimulatorApp({ title, navItems, children }: SimulatorAppProps) {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AppShell title={title} navItems={navItems} renderLink={renderNavLink}>
          <Routes>{children}</Routes>
        </AppShell>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
