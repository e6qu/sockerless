/** Renders a row of colored label pills for GitHub issues. */
export function LabelPills({ labels }: { labels?: { name: string; color: string }[] }) {
  if (!labels || labels.length === 0) return null;
  return (
    <>
      {labels.map((l) => (
        <span
          key={l.name}
          style={{
            padding: "0.1rem 0.55rem",
            borderRadius: "2rem",
            fontSize: "0.72rem",
            fontWeight: 500,
            background: `#${l.color}22`,
            color: `#${l.color}`,
            border: `1px solid #${l.color}55`,
            whiteSpace: "nowrap",
          }}
        >
          {l.name}
        </span>
      ))}
    </>
  );
}
