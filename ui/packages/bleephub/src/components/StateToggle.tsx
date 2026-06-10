import type { ReactNode } from "react";

/** Open/closed segmented filter for list pages (Issues, PRs). */
export function StateToggle<T extends string>({
  value,
  options,
  labels,
  icons,
  onChange,
}: {
  value: T;
  options: readonly T[];
  labels: Record<T, string>;
  icons?: Partial<Record<T, ReactNode>>;
  onChange: (v: T) => void;
}) {
  return (
    <div
      className="inline-flex items-center"
      style={{
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        overflow: "hidden",
      }}
    >
      {options.map((opt, i) => {
        const active = value === opt;
        return (
          <button
            key={opt}
            onClick={() => onChange(opt)}
            className="inline-flex items-center gap-1.5"
            style={{
              padding: "0.35rem 0.75rem",
              fontSize: "0.82rem",
              fontWeight: active ? 600 : 500,
              color: active ? "var(--color-fg)" : "var(--color-fg-muted)",
              background: active ? "var(--color-bg-subtle)" : "transparent",
              border: "none",
              borderLeft: i > 0 ? "1px solid var(--color-border)" : "none",
            }}
          >
            {icons?.[opt]}
            {labels[opt]}
          </button>
        );
      })}
    </div>
  );
}
