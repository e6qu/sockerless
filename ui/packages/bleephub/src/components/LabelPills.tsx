/** Renders a row of colored label pills for GitHub issues. */
export function LabelPills({ labels }: { labels?: { name: string; color: string }[] }) {
  if (!labels || labels.length === 0) return null;
  return (
    <>
      {labels.map((l) => (
        <span
          key={l.name}
          style={{
            padding: "0.15rem 0.5rem",
            borderRadius: "1rem",
            fontSize: "0.7rem",
            fontFamily: "var(--font-mono)",
            background: `#${l.color}22`,
            color: `#${l.color}`,
            border: `1px solid #${l.color}44`,
            whiteSpace: "nowrap",
          }}
        >
          {l.name}
        </span>
      ))}
    </>
  );
}
