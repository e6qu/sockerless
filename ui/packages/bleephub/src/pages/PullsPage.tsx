import { useState } from "react";
import { useParams, Link, useNavigate } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { PageHeading, Spinner, Button } from "@sockerless/ui-core/components";
import { fetchRepoPRs, fetchIssueComments, mergePR } from "../api.js";

export function PullsPage() {
  const { owner = "", repo = "", number } = useParams<{
    owner: string;
    repo: string;
    number?: string;
  }>();

  if (number) {
    return <PRDetail owner={owner} repo={repo} number={parseInt(number, 10)} />;
  }
  return <PRList owner={owner} repo={repo} />;
}

function PRList({ owner, repo }: { owner: string; repo: string }) {
  const [state, setState] = useState<"open" | "closed">("open");

  const { data: prs = [], isLoading } = useQuery({
    queryKey: ["prs", owner, repo, state],
    queryFn: () => fetchRepoPRs(owner, repo, state),
    enabled: !!owner && !!repo,
  });

  if (isLoading) return <Spinner label="loading pull requests" />;

  return (
    <div>
      <div style={{ marginBottom: "0.25rem" }}>
        <Link
          to={`/ui/repos/${owner}/${repo}`}
          style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem", fontFamily: "var(--font-mono)" }}
        >
          ← {owner}/{repo}
        </Link>
      </div>
      <PageHeading
        kicker={`${owner}/${repo}`}
        title={<>Pull Requests</>}
        meta={`${prs.length} ${state} PR${prs.length !== 1 ? "s" : ""}`}
      />

      {/* State toggle */}
      <div style={{ display: "flex", gap: "0.5rem", marginBottom: "1rem" }}>
        {(["open", "closed"] as const).map((s) => (
          <button
            key={s}
            onClick={() => setState(s)}
            style={{
              padding: "0.3rem 0.75rem",
              fontSize: "0.8rem",
              fontFamily: "var(--font-mono)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-sm)",
              background: state === s ? "var(--color-accent-soft)" : "transparent",
              color: state === s ? "var(--color-accent)" : "var(--color-fg-muted)",
              cursor: "pointer",
            }}
          >
            {s === "open" ? "↯ Open" : "✓ Merged / Closed"}
          </button>
        ))}
      </div>

      {prs.length === 0 ? (
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
          No {state} pull requests.
        </div>
      ) : (
        <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
          {prs.map((pr, i) => (
            <Link
              key={pr.id}
              to={`/ui/repos/${owner}/${repo}/pulls/${pr.number}`}
              style={{
                display: "flex",
                alignItems: "flex-start",
                gap: "0.75rem",
                padding: "0.9rem 1rem",
                borderBottom: i < prs.length - 1 ? "1px solid var(--color-border)" : "none",
                textDecoration: "none",
                background: "var(--color-surface-raised)",
                transition: "background 0.1s",
              }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.background = "var(--color-bg-subtle)"; }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.background = "var(--color-surface-raised)"; }}
            >
              <span
                style={{
                  marginTop: "0.15rem",
                  color: pr.state === "open" ? "var(--color-status-ok)" : pr.merged ? "var(--color-status-info)" : "var(--color-status-error)",
                  fontSize: "1rem",
                }}
              >
                {pr.merged ? "⊕" : pr.state === "open" ? "↯" : "✗"}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>
                  {pr.title}
                  {pr.draft && (
                    <span style={{ marginLeft: "0.5rem", fontSize: "0.72rem", color: "var(--color-fg-subtle)", fontFamily: "var(--font-mono)" }}>
                      [draft]
                    </span>
                  )}
                </div>
                <div style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
                  #{pr.number} · <span style={{ color: "var(--color-accent)" }}>{pr.head.ref}</span>
                  {" → "}<span style={{ color: "var(--color-fg-muted)" }}>{pr.base.ref}</span>
                  {" · opened by "}{pr.user?.login}
                  {" · "}{new Date(pr.created_at).toLocaleDateString()}
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

function PRDetail({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const { data: prs = [] } = useQuery({
    queryKey: ["prs", owner, repo, "all"],
    queryFn: () => fetchRepoPRs(owner, repo, "all"),
  });
  const pr = prs.find((p) => p.number === number);
  const { data: comments = [] } = useQuery({
    queryKey: ["pr-comments", owner, repo, number],
    queryFn: () => fetchIssueComments(owner, repo, number),
    enabled: !!pr,
  });
  const qc = useQueryClient();
  const navigate = useNavigate();

  const mergeMutation = useMutation({
    mutationFn: () => mergePR(owner, repo, number),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["prs", owner, repo] });
      navigate(`/ui/repos/${owner}/${repo}/pulls`);
    },
  });

  if (!pr) return <Spinner label={`loading PR #${number}`} />;

  const isMergeable = pr.state === "open" && !pr.draft && pr.merged_at === null;

  return (
    <div>
      <div style={{ marginBottom: "0.5rem" }}>
        <Link
          to={`/ui/repos/${owner}/${repo}/pulls`}
          style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem", fontFamily: "var(--font-mono)" }}
        >
          ← Pull Requests
        </Link>
      </div>
      <PageHeading
        kicker={`${owner}/${repo} · PR #${pr.number}`}
        title={<>{pr.title}</>}
        meta={
          `${pr.state} · ${pr.head.ref} → ${pr.base.ref} · opened by ${pr.user?.login} · ${new Date(pr.created_at).toLocaleDateString()}`
        }
        actions={
          isMergeable ? (
            <Button
              variant="primary"
              size="sm"
              disabled={mergeMutation.isPending}
              onClick={() => mergeMutation.mutate()}
            >
              {mergeMutation.isPending ? "Merging…" : "Merge pull request"}
            </Button>
          ) : pr.merged ? (
            <span style={{ fontSize: "0.8rem", color: "var(--color-status-info)", fontFamily: "var(--font-mono)" }}>
              ⊕ Merged
            </span>
          ) : null
        }
      />

      {/* PR body */}
      <div
        style={{
          border: "1px solid var(--color-border)",
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
          <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{pr.user?.login}</span>
          <span>opened {new Date(pr.created_at).toLocaleString()}</span>
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
          {pr.body || <span style={{ color: "var(--color-fg-muted)" }}>No description.</span>}
        </div>
      </div>

      {/* Comments */}
      {comments.map((c) => (
        <div
          key={c.id}
          style={{
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-md)",
            marginBottom: "0.75rem",
            overflow: "hidden",
          }}
        >
          <div
            style={{
              padding: "0.5rem 0.85rem",
              background: "var(--color-bg-subtle)",
              borderBottom: "1px solid var(--color-border)",
              fontSize: "0.75rem",
              fontFamily: "var(--font-mono)",
              color: "var(--color-fg-muted)",
            }}
          >
            <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{c.user?.login}</span>
            {" · "}{new Date(c.created_at).toLocaleString()}
          </div>
          <div style={{ padding: "0.75rem 1rem", fontSize: "0.875rem", lineHeight: 1.6, color: "var(--color-fg)", whiteSpace: "pre-wrap" }}>
            {c.body}
          </div>
        </div>
      ))}
    </div>
  );
}
