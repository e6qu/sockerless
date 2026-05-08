import { type ReactNode } from "react";

export interface PageHeadingProps {
  /** Small label printed above the title — typically a section/category. */
  kicker?: string;
  /** The headline. Rendered in the serif display voice. */
  title: ReactNode;
  /** Sub-line under the title (monospace). */
  meta?: ReactNode;
  /** Right-aligned actions / button row. */
  actions?: ReactNode;
}

/**
 * Editorial-magazine page header: a small uppercase kicker over a
 * large italic-serif title, optional monospace meta beneath, and a
 * right-aligned actions slot. Sets the voice for every page.
 */
export function PageHeading({ kicker, title, meta, actions }: PageHeadingProps) {
  return (
    <header className="mb-8 flex flex-wrap items-end justify-between gap-x-8 gap-y-3 border-b pb-5"
      style={{ borderColor: "var(--color-border)" }}
    >
      <div className="min-w-0 flex-1">
        {kicker && (
          <div
            className="mb-2 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            {kicker}
          </div>
        )}
        <h2
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "clamp(1.6rem, 2.4vw, 2.4rem)",
            lineHeight: 1.05,
            letterSpacing: "-0.025em",
            color: "var(--color-fg)",
          }}
        >
          {title}
        </h2>
        {meta && (
          <div
            className="mt-2 text-xs font-mono"
            style={{ color: "var(--color-fg-muted)" }}
          >
            {meta}
          </div>
        )}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}
