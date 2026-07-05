import type { ReactNode } from "react";

/*
 * GitHub-style settings shell: a LEFT vertical sub-navigation with an
 * adjacent content pane on the right (not a horizontal tab strip). Nav
 * items are grouped into optionally-titled sections, mirroring GitHub's
 * repo/account settings sidebars. The selected item is a plain <button>
 * so it stays keyboard- and screen-reader-accessible.
 */

export interface SettingsNavItem<K extends string> {
  key: K;
  label: ReactNode;
  icon?: ReactNode;
}

export interface SettingsNavSection<K extends string> {
  /** Optional group heading rendered above the section's items. */
  title?: string;
  items: SettingsNavItem<K>[];
}

export function SettingsLayout<K extends string>({
  sections,
  active,
  onSelect,
  children,
}: {
  sections: SettingsNavSection<K>[];
  active: K;
  onSelect: (key: K) => void;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-start gap-6 md:flex-nowrap">
      <aside className="w-full shrink-0 md:w-56">
        <nav aria-label="Settings" className="flex flex-col gap-4">
          {sections.map((section, i) => (
            <div key={section.title ?? `section-${i}`}>
              {section.title && (
                <div
                  className="mb-1 px-2"
                  style={{
                    fontSize: "0.72rem",
                    fontWeight: 600,
                    textTransform: "uppercase",
                    letterSpacing: "0.04em",
                    color: "var(--color-fg-muted)",
                  }}
                >
                  {section.title}
                </div>
              )}
              <ul className="flex flex-col" style={{ listStyle: "none", margin: 0, padding: 0 }}>
                {section.items.map((item) => {
                  const selected = item.key === active;
                  return (
                    <li key={item.key}>
                      <button
                        type="button"
                        aria-current={selected ? "page" : undefined}
                        onClick={() => onSelect(item.key)}
                        className="flex w-full items-center gap-2 text-left"
                        style={{
                          padding: "0.35rem 0.6rem",
                          marginBottom: "1px",
                          fontSize: "0.85rem",
                          fontWeight: selected ? 600 : 500,
                          color: "var(--color-fg)",
                          background: selected
                            ? "color-mix(in srgb, var(--color-fg-muted) 12%, transparent)"
                            : "transparent",
                          border: "none",
                          borderLeft: `2px solid ${selected ? "var(--color-accent)" : "transparent"}`,
                          borderRadius: "var(--radius-sm)",
                          cursor: "pointer",
                        }}
                      >
                        {item.icon && (
                          <span style={{ color: "var(--color-fg-muted)", flexShrink: 0 }}>
                            {item.icon}
                          </span>
                        )}
                        <span className="min-w-0 flex-1 truncate">{item.label}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>
      </aside>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
