import { type ReactNode } from "react";

export interface NavItem {
  label: string;
  to: string;
}

export interface AppShellProps {
  title: string;
  navItems: NavItem[];
  renderLink: (item: NavItem, isActive?: boolean) => ReactNode;
  children: ReactNode;
}

export function AppShell({ title, navItems, renderLink, children }: AppShellProps) {
  return (
    <div className="flex h-screen bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100">
      {/* Sidebar */}
      <aside className="w-56 flex-shrink-0 border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="p-4 border-b border-gray-200 dark:border-gray-700">
          <h1 className="text-lg font-semibold truncate">{title}</h1>
        </div>
        <nav className="p-2 space-y-1">
          {navItems.map((item) => (
            <div key={item.to}>{renderLink(item)}</div>
          ))}
        </nav>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  );
}
