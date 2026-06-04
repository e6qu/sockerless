import type { GithubComment } from "../types.js";

export interface CommentCardProps {
  login?: string;
  body?: string;
  date: string;
  isOp?: boolean;
}

export function CommentCard({ login, body, date, isOp = false }: CommentCardProps) {
  return (
    <div
      style={{
        border: `1px solid ${isOp ? "var(--color-accent)" : "var(--color-border)"}`,
        borderRadius: "var(--radius-md)",
        marginBottom: "1rem",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "0.5rem",
          padding: "0.6rem 0.85rem",
          background: "var(--color-bg-subtle)",
          borderBottom: "1px solid var(--color-border)",
          fontSize: "0.78rem",
          fontFamily: "var(--font-mono)",
          color: "var(--color-fg-muted)",
        }}
      >
        <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{login}</span>
        <span>commented {new Date(date).toLocaleString()}</span>
        {isOp && (
          <span
            style={{
              marginLeft: "auto",
              padding: "0.1rem 0.4rem",
              border: "1px solid var(--color-accent)",
              borderRadius: "var(--radius-sm)",
              fontSize: "0.68rem",
              color: "var(--color-accent)",
            }}
          >
            Author
          </span>
        )}
      </div>
      <div
        style={{
          padding: "0.85rem 1rem",
          fontSize: "0.875rem",
          lineHeight: 1.6,
          color: "var(--color-fg)",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {body || <span style={{ color: "var(--color-fg-muted)" }}>No description.</span>}
      </div>
    </div>
  );
}

/** Map an array of GithubComment objects to CommentCard elements. */
export function CommentList({ comments }: { comments: GithubComment[] }) {
  return (
    <>
      {comments.map((c) => (
        <CommentCard
          key={c.id}
          login={c.user?.login}
          body={c.body}
          date={c.created_at}
        />
      ))}
    </>
  );
}
