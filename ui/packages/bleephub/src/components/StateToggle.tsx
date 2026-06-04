/** State toggle for list pages (open/closed, etc.). */
export function StateToggle<T extends string>({
  value,
  options,
  labels,
  onChange,
}: {
  value: T;
  options: readonly T[];
  labels: Record<T, string>;
  onChange: (v: T) => void;
}) {
  return (
    <div style={{ display: "flex", gap: "0.5rem", marginBottom: "1rem" }}>
      {options.map((opt) => (
        <button
          key={opt}
          onClick={() => onChange(opt)}
          style={{
            padding: "0.3rem 0.75rem",
            fontSize: "0.8rem",
            fontFamily: "var(--font-mono)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-sm)",
            background: value === opt ? "var(--color-accent-soft)" : "transparent",
            color: value === opt ? "var(--color-accent)" : "var(--color-fg-muted)",
            cursor: "pointer",
          }}
        >
          {labels[opt]}
        </button>
      ))}
    </div>
  );
}

/** Centered dashed-border empty state for list pages. */
export function EmptyListPlaceholder({ message }: { message: string }) {
  return (
    <div
      style={{
        padding: "2.5rem",
        textAlign: "center",
        border: "1px dashed var(--color-border)",
        borderRadius: "var(--radius-md)",
        color: "var(--color-fg-muted)",
        fontFamily: "var(--font-mono)",
        fontSize: "0.85rem",
      }}
    >
      {message}
    </div>
  );
}
