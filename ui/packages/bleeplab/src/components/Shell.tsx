import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";
import { ThemeToggle } from "@sockerless/ui-core/components";

const nav = [
  { label: "Overview", to: "/ui/" },
  { label: "Projects", to: "/ui/projects" },
  { label: "Pipelines", to: "/ui/pipelines" },
  { label: "Runners", to: "/ui/runners" },
];

function active(pathname: string, to: string): boolean {
  if (to === "/ui/") return pathname === "/ui/" || pathname === "/ui";
  return pathname === to || pathname.startsWith(to + "/");
}

/** TanukiMark — a simple GitLab-tanuki-orange brand glyph (not the official
 *  logo; an evocative three-bar mark in the brand orange). */
function TanukiMark() {
  return (
    <span
      aria-hidden
      className="inline-flex items-end gap-[2px]"
      style={{ height: 18 }}
    >
      <span style={{ width: 4, height: 10, background: "var(--gl-orange-strong)" }} />
      <span style={{ width: 4, height: 18, background: "var(--gl-orange)" }} />
      <span style={{ width: 4, height: 10, background: "var(--gl-orange-strong)" }} />
    </span>
  );
}

export function BleeplabShell({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  return (
    <div
      className="grid h-screen w-full"
      style={{ gridTemplateColumns: "minmax(220px, 15rem) 1fr" }}
    >
      <aside
        className="flex flex-col gap-1 overflow-y-auto px-3 py-4"
        style={{
          background: "var(--color-bg-subtle)",
          borderRight: "1px solid var(--color-border)",
        }}
      >
        <Link
          to="/ui/"
          className="mb-4 flex items-center gap-2.5 px-2"
          style={{ color: "var(--color-fg)", textDecoration: "none" }}
        >
          <TanukiMark />
          <span className="flex flex-col leading-tight">
            <span className="font-display text-[15px] font-semibold">bleeplab</span>
            <span
              className="text-[10px] uppercase tracking-[0.18em]"
              style={{ color: "var(--color-fg-subtle)" }}
            >
              GitLab control plane
            </span>
          </span>
        </Link>

        <nav className="flex flex-col gap-0.5">
          {nav.map((item) => {
            const on = active(pathname, item.to);
            return (
              <Link
                key={item.to}
                to={item.to}
                className="rounded px-2.5 py-1.5 text-sm"
                style={{
                  textDecoration: "none",
                  fontWeight: on ? 600 : 400,
                  color: on ? "var(--color-accent)" : "var(--color-fg-muted)",
                  background: on ? "var(--color-accent-soft)" : "transparent",
                  borderRadius: "var(--radius-sm)",
                }}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="mt-auto flex items-center justify-between px-1 pt-4">
          <span className="text-[10px]" style={{ color: "var(--color-fg-subtle)" }}>
            sockerless sim
          </span>
          <ThemeToggle />
        </div>
      </aside>

      <main
        id="main-content"
        className="overflow-y-auto px-8 py-8"
        style={{ background: "var(--color-bg)" }}
      >
        <div className="mx-auto w-full max-w-6xl">{children}</div>
      </main>
    </div>
  );
}
