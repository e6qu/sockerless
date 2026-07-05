import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
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
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        marginBottom: "1rem",
        overflow: "hidden",
      }}
    >
      <div
        className="flex items-center gap-2"
        style={{
          padding: "0.5rem 0.85rem",
          background: "var(--color-bg-subtle)",
          borderBottom: "1px solid var(--color-border)",
          fontSize: "0.82rem",
          color: "var(--color-fg-muted)",
        }}
      >
        <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{login}</span>
        <span>commented {new Date(date).toLocaleString()}</span>
        {isOp && (
          <span
            style={{
              marginLeft: "auto",
              padding: "0.05rem 0.45rem",
              border: "1px solid var(--color-border)",
              borderRadius: "2rem",
              fontSize: "0.7rem",
              color: "var(--color-fg-muted)",
            }}
          >
            Author
          </span>
        )}
      </div>
      <div
        className={body ? "markdown-body" : undefined}
        style={{
          padding: "0.85rem 1rem",
          fontSize: "0.9rem",
          lineHeight: 1.6,
          color: "var(--color-fg)",
          wordBreak: "break-word",
        }}
      >
        {body ? (
          <Markdown remarkPlugins={[remarkGfm]}>{body}</Markdown>
        ) : (
          <span style={{ color: "var(--color-fg-muted)" }}>No description provided.</span>
        )}
      </div>
    </div>
  );
}

/** Map an array of GithubComment objects to CommentCard elements. */
export function CommentList({ comments }: { comments: GithubComment[] }) {
  return (
    <>
      {comments.map((c) => (
        <CommentCard key={c.id} login={c.user?.login} body={c.body} date={c.created_at} />
      ))}
    </>
  );
}
