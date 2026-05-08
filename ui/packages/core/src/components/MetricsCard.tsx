export interface MetricsCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  /** Optional emphasis — pulls the number into the accent colour. */
  emphasized?: boolean;
}

/**
 * Editorial metric panel. The label is small monospace caps, the
 * number is the serif display voice — pulled large to sit like a pull
 * quote. A 2-px accent bar runs down the left side of the panel
 * (mimics the AppShell rail at smaller scale, ties everything together).
 */
export function MetricsCard({ title, value, subtitle, emphasized }: MetricsCardProps) {
  return (
    <div
      className="relative px-4 py-4"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderLeft: emphasized
          ? "3px solid var(--color-accent)"
          : "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div
        className="text-[10px] uppercase tracking-[0.2em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {title}
      </div>
      <div
        className="mt-2 font-display tabular-nums"
        style={{
          fontSize: "1.85rem",
          fontWeight: 600,
          fontStyle: "italic",
          letterSpacing: "-0.02em",
          lineHeight: 1,
          color: emphasized ? "var(--color-accent)" : "var(--color-fg)",
        }}
      >
        {value}
      </div>
      {subtitle && (
        <div
          className="mt-2 text-[11px] font-mono"
          style={{ color: "var(--color-fg-muted)" }}
        >
          {subtitle}
        </div>
      )}
    </div>
  );
}
