import { type ReactNode } from "react";
import { ThemeToggle } from "./ThemeToggle.js";

export interface NavItem {
  label: string;
  to: string;
}

export interface AppShellProps {
  title: string;
  /** Optional small label printed above the title. Acts as a kicker. */
  kicker?: string;
  navItems: NavItem[];
  renderLink: (item: NavItem, isActive?: boolean) => ReactNode;
  children: ReactNode;
}

/**
 * Editorial-brutalist shell. Left sidebar carries the brand mark (kicker
 * + serif title) above a tight monospace nav. A vertical accent rail
 * sits between sidebar and content so each app's identity announces
 * itself without needing a logo. Content area dropped onto a textured
 * page (the dotted grid is in the global stylesheet).
 */
export function AppShell({ title, kicker, navItems, renderLink, children }: AppShellProps) {
  return (
    <div
      className="grid h-screen w-full"
      style={{ gridTemplateColumns: "minmax(220px, 16rem) 4px 1fr" }}
    >
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only"
        style={{
          position: "absolute",
          top: 8,
          left: 8,
          padding: "0.4rem 0.75rem",
          background: "var(--color-accent)",
          color: "var(--color-accent-fg)",
          fontSize: "0.78rem",
          zIndex: 100,
          borderRadius: "var(--radius-sm)",
        }}
      >
        Skip to main content
      </a>
      <aside
        aria-label="Sidebar"
        className="flex h-full flex-col overflow-hidden border-r"
        style={{
          background: "color-mix(in oklch, var(--color-bg) 80%, var(--color-surface))",
          borderColor: "var(--color-border)",
        }}
      >
        <div
          className="flex flex-col gap-1 border-b px-5 py-6"
          style={{ borderColor: "var(--color-border)" }}
        >
          {kicker && (
            <div
              className="text-[10px] uppercase tracking-[0.18em]"
              style={{ color: "var(--color-fg-subtle)" }}
            >
              {kicker}
            </div>
          )}
          <h1
            className="text-2xl font-display"
            style={{
              fontStyle: "italic",
              fontWeight: 600,
              color: "var(--color-fg)",
              letterSpacing: "-0.02em",
              lineHeight: 1,
            }}
          >
            {title}
          </h1>
        </div>

        <nav aria-label="Primary" className="flex-1 overflow-y-auto px-3 py-4">
          <ul className="space-y-0.5">
            {navItems.map((item, i) => (
              <li
                key={item.to}
                className="reveal"
                style={{ "--reveal-delay": `${i * 30}ms` } as React.CSSProperties}
              >
                {renderLink(item)}
              </li>
            ))}
          </ul>
        </nav>

        <div
          className="flex items-center justify-between gap-2 border-t px-5 py-3 text-[10px] uppercase tracking-[0.2em]"
          style={{
            borderColor: "var(--color-border)",
            color: "var(--color-fg-subtle)",
          }}
        >
          <span>sockerless · operator</span>
          <ThemeToggle />
        </div>
      </aside>

      {/* Vertical accent rail — the brand statement. Per-app accent
       * colour means each tool announces itself the second the page
       * loads, no logo required. */}
      <div style={{ background: "var(--color-accent)" }} aria-hidden />

      <main
        id="main-content"
        tabIndex={-1}
        className="overflow-auto px-8 py-8"
        style={{
          background: "var(--color-bg)",
          color: "var(--color-fg)",
        }}
      >
        <div className="mx-auto max-w-[1400px]">{children}</div>
      </main>
    </div>
  );
}

/**
 * Default nav-link renderer. Apps can pass their own to AppShell to
 * customise; this one is monospace, sharp, and carries the accent
 * colour as a left rule when active.
 */
export interface NavLinkButtonProps {
  active: boolean;
  children: ReactNode;
}

export function NavLinkButton({ active, children }: NavLinkButtonProps) {
  return (
    <span
      className="block px-3 py-1.5 text-[13px] font-mono"
      style={{
        color: active ? "var(--color-fg)" : "var(--color-fg-muted)",
        background: active ? "var(--color-bg)" : "transparent",
        borderLeft: `2px solid ${active ? "var(--color-accent)" : "transparent"}`,
        transition: "all 0.12s var(--ease-out-quint)",
      }}
    >
      {children}
    </span>
  );
}
