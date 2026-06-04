import { useState } from "react";
import { useParams, Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeading, Spinner, Button, InlineError } from "@sockerless/ui-core/components";
import { fetchRepoIssues, fetchIssueDetail, fetchIssueComments, createIssue } from "../api.js";
import { useRepoItemList } from "../hooks/useRepoItemList.js";
import type { GithubIssue } from "../types.js";
import { CommentCard, CommentList } from "../components/CommentCard.js";
import { rowHoverProps } from "../components/RowHover.js";
import { EmptyListPlaceholder } from "../components/StateToggle.js";
import { LabelPills } from "../components/LabelPills.js";
import { ListPageHeader } from "../components/ListPageHeader.js";

export function IssuesPage() {
  const { owner = "", repo = "", number } = useParams<{
    owner: string;
    repo: string;
    number?: string;
  }>();

  if (number) {
    return <IssueDetail owner={owner} repo={repo} number={parseInt(number, 10)} />;
  }
  return <IssueList owner={owner} repo={repo} />;
}

function IssueList({ owner, repo }: { owner: string; repo: string }) {
  const { state, setState, items: issues, isLoading, isError, error } = useRepoItemList(
    "issues", owner, repo, fetchRepoIssues,
  );
  const [creating, setCreating] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newBody, setNewBody] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();

  const mutation = useMutation({
    mutationFn: () => createIssue(owner, repo, { title: newTitle, body: newBody }),
    onSuccess: (issue: GithubIssue) => {
      qc.invalidateQueries({ queryKey: ["issues", owner, repo] });
      setCreating(false);
      setNewTitle("");
      setNewBody("");
      navigate(`/ui/repos/${owner}/${repo}/issues/${issue.number}`);
    },
  });

  if (isLoading) return <Spinner label="loading issues" />;
  if (isError) return <InlineError title="Failed to load issues" detail={String(error)} />;

  return (
    <div>
      <ListPageHeader
        owner={owner}
        repo={repo}
        backTo={`/ui/repos/${owner}/${repo}`}
        title={<>Issues</>}
        meta={`${issues.length} ${state} issue${issues.length !== 1 ? "s" : ""}`}
        actions={<Button variant="primary" size="sm" onClick={() => setCreating(true)}>New issue</Button>}
        state={state}
        stateLabels={{ open: "○ Open", closed: "✓ Closed" }}
        onStateChange={setState}
      />

      {/* Create issue modal */}
      {creating && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.5)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 50,
          }}
          onClick={(e) => e.target === e.currentTarget && setCreating(false)}
        >
          <div
            style={{
              background: "var(--color-surface-raised)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-md)",
              padding: "1.5rem",
              width: "min(560px, 90vw)",
            }}
          >
            <h2 style={{ fontFamily: "var(--font-display)", fontSize: "1.1rem", marginBottom: "1rem" }}>
              New issue
            </h2>
            <div style={{ marginBottom: "0.75rem" }}>
              <label style={{ display: "block", fontSize: "0.8rem", fontFamily: "var(--font-mono)", color: "var(--color-fg-muted)", marginBottom: "0.3rem" }}>
                Title
              </label>
              <input
                autoFocus
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                placeholder="Issue title"
                style={{
                  width: "100%",
                  padding: "0.5rem 0.75rem",
                  background: "var(--color-bg)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "var(--radius-sm)",
                  color: "var(--color-fg)",
                  fontFamily: "var(--font-mono)",
                  fontSize: "0.85rem",
                  boxSizing: "border-box",
                }}
              />
            </div>
            <div style={{ marginBottom: "1rem" }}>
              <label style={{ display: "block", fontSize: "0.8rem", fontFamily: "var(--font-mono)", color: "var(--color-fg-muted)", marginBottom: "0.3rem" }}>
                Description (optional)
              </label>
              <textarea
                value={newBody}
                onChange={(e) => setNewBody(e.target.value)}
                rows={4}
                placeholder="Describe the issue…"
                style={{
                  width: "100%",
                  padding: "0.5rem 0.75rem",
                  background: "var(--color-bg)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "var(--radius-sm)",
                  color: "var(--color-fg)",
                  fontFamily: "var(--font-mono)",
                  fontSize: "0.85rem",
                  resize: "vertical",
                  boxSizing: "border-box",
                }}
              />
            </div>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: "0.5rem" }}>
              <Button variant="ghost" size="sm" onClick={() => setCreating(false)}>Cancel</Button>
              <Button
                variant="primary"
                size="sm"
                disabled={!newTitle.trim() || mutation.isPending}
                onClick={() => mutation.mutate()}
              >
                {mutation.isPending ? "Creating…" : "Create issue"}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Issue list */}
      {issues.length === 0 ? (
        <EmptyListPlaceholder message={`No ${state} issues.`} />
      ) : (
        <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
          {issues.map((issue, i) => (
            <Link
              key={issue.id}
              to={`/ui/repos/${owner}/${repo}/issues/${issue.number}`}
              style={{
                display: "flex",
                alignItems: "flex-start",
                gap: "0.75rem",
                padding: "0.9rem 1rem",
                borderBottom: i < issues.length - 1 ? "1px solid var(--color-border)" : "none",
                textDecoration: "none",
                background: "var(--color-surface-raised)",
                transition: "background 0.1s",
              }}
              {...rowHoverProps}
            >
              <span
                style={{
                  marginTop: "0.15rem",
                  color: issue.state === "open" ? "var(--color-status-ok)" : "var(--color-fg-muted)",
                  fontSize: "1rem",
                }}
              >
                {issue.state === "open" ? "○" : "✓"}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>
                  {issue.title}
                </div>
                <div style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", marginTop: "0.25rem" }}>
                  #{issue.number} opened by {issue.user?.login} ·{" "}
                  {new Date(issue.created_at).toLocaleDateString()}
                  {issue.comments > 0 && ` · ${issue.comments} comments`}
                </div>
              </div>
              <div style={{ display: "flex", gap: "0.3rem", flexWrap: "wrap" }}>
                <LabelPills labels={issue.labels} />
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

function IssueDetail({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const { data: issue, isLoading } = useQuery({
    queryKey: ["issue", owner, repo, number],
    queryFn: () => fetchIssueDetail(owner, repo, number),
  });
  const { data: comments = [] } = useQuery({
    queryKey: ["issue-comments", owner, repo, number],
    queryFn: () => fetchIssueComments(owner, repo, number),
  });

  if (isLoading || !issue) return <Spinner label={`loading issue #${number}`} />;

  return (
    <div>
      <div style={{ marginBottom: "0.5rem" }}>
        <Link
          to={`/ui/repos/${owner}/${repo}/issues`}
          style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem", fontFamily: "var(--font-mono)" }}
        >
          ← Issues
        </Link>
      </div>
      <PageHeading
        kicker={`${owner}/${repo} · issue #${issue.number}`}
        title={<>{issue.title}</>}
        meta={
          `${issue.state === "open" ? "Open" : "Closed"} · opened by ${issue.user?.login} · ${new Date(issue.created_at).toLocaleDateString()}`
        }
      />

      {/* Original post */}
      <CommentCard
        login={issue.user?.login}
        body={issue.body}
        date={issue.created_at}
        isOp
      />

      {/* Comments */}
      <CommentList comments={comments} />
      {comments.length === 0 && (
        <div style={{ padding: "1rem 0", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.82rem" }}>
          No comments yet.
        </div>
      )}
    </div>
  );
}
