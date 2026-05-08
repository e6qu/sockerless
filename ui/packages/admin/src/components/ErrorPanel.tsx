/**
 * Inline error banner used by every admin query page. Editorial-
 * brutalist treatment matching the design system: kicker + monospace
 * message, error left-bar, no rounded blob.
 */
export interface ErrorPanelProps {
  message?: string;
  /** Optional kicker label (e.g. "fetch failed"). */
  kicker?: string;
}

export function ErrorPanel({ message, kicker = "error" }: ErrorPanelProps) {
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
      {message ?? "request failed"}
    </div>
  );
}
