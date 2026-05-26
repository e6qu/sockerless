/**
 * Inline error banner used by every admin query page. Editorial-
 * brutalist treatment matching the design system: kicker + monospace
 * message, error left-bar, no rounded blob.
 */
export interface ErrorPanelProps {
  message?: string;
  /** Optional kicker label (e.g. "fetch failed"). */
  kicker?: string;
  /** Operator commands that recover this failure path. */
  commands?: string[];
}

export function ErrorPanel({
  message,
  kicker = "error",
  commands = [],
}: ErrorPanelProps) {
  return (
    <div
      className="px-4 py-3 font-mono"
      style={{
        background: "var(--color-status-error-soft)",
        color: "var(--color-status-error)",
        border: "1px solid var(--color-status-error)",
        borderLeft: "3px solid var(--color-status-error)",
        borderRadius: "var(--radius-sm)",
        fontSize: "0.78rem",
      }}
    >
      <div
        className="text-[10px] uppercase tracking-[0.22em] mb-0.5"
        style={{ opacity: 0.85 }}
      >
        {kicker}
      </div>
      <div>{message ?? "request failed"}</div>
      {commands.length > 0 && (
        <div
          className="mt-3"
          style={{ color: "var(--color-fg)", fontSize: "0.76rem" }}
        >
          {commands.map((cmd) => (
            <code
              key={cmd}
              className="mb-1 block"
              style={{
                background: "var(--color-bg)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-xs)",
                padding: "0.35rem 0.5rem",
                whiteSpace: "pre-wrap",
              }}
            >
              {cmd}
            </code>
          ))}
        </div>
      )}
    </div>
  );
}
