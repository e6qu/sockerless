import { useState } from "react";
import { useParams, Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoIssuesPage,
  fetchIssueDetail,
  fetchIssueComments,
  createIssue,
  isNotFound,
} from "../api.js";
import { useRepoItemList } from "../hooks/useRepoItemList.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import type { GithubIssue } from "../types.js";
import { CommentCard, CommentList } from "../components/CommentCard.js";
import { LabelPills } from "../components/LabelPills.js";
import { StateToggle } from "../components/StateToggle.js";
import { RepoHeader } from "../components/Shell.js";
import {
  Button,
  Box,
  Blankslate,
  StateLabel,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
} from "../components/ui.js";
import { IssueOpenedIcon, IssueClosedIcon, CommentIcon } from "../components/octicons.js";

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
  const {
    state,
    setState,
    items: issues,
    isLoading,
    isError,
    error,
    hasMore,
    loadMore,
    isLoadingMore,
  } = useRepoItemList("issues", owner, repo, fetchRepoIssuesPage);
  const counts = useOpenCounts(owner, repo);
  const [creating, setCreating] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newBody, setNewBody] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();

  const [createError, setCreateError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => createIssue(owner, repo, { title: newTitle, body: newBody }),
    onSuccess: (issue: GithubIssue) => {
      qc.invalidateQueries({ queryKey: ["issues", owner, repo] });
      setCreating(false);
      setNewTitle("");
      setNewBody("");
      setCreateError(null);
      navigate(`/ui/repos/${owner}/${repo}/issues/${issue.number}`);
    },
    onError: (err: Error) => setCreateError(err.message),
  });

  if (isLoading) return <Spinner label="loading issues" />;
  if (isError) return <InlineError title="Failed to load issues" detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="issues" {...counts} />

      <div className="mb-4 flex items-center justify-between gap-3">
        <StateToggle
          value={state}
          options={["open", "closed"] as const}
          labels={{ open: "Open", closed: "Closed" }}
          icons={{ open: <IssueOpenedIcon size={14} />, closed: <IssueClosedIcon size={14} /> }}
          onChange={setState}
        />
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New issue
        </Button>
      </div>

      {creating && (
        <Modal title="New issue" onClose={() => setCreating(false)}>
          <FormLabel id="issue-title">Title</FormLabel>
          <input
            id="issue-title"
            autoFocus
            value={newTitle}
            onChange={(e) => setNewTitle(e.target.value)}
            placeholder="Issue title"
            className="mb-3 w-full"
          />
          <FormLabel id="issue-body">Description (optional)</FormLabel>
          <textarea
            id="issue-body"
            value={newBody}
            onChange={(e) => setNewBody(e.target.value)}
            rows={5}
            placeholder="Describe the issue…"
            className="mb-4 w-full"
            style={{ resize: "vertical" }}
          />
          {createError && <ErrorBanner>{createError}</ErrorBanner>}
          <DialogActions>
            <Button variant="ghost" size="sm" onClick={() => setCreating(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={!newTitle.trim() || mutation.isPending}
              onClick={() => {
                setCreateError(null);
                mutation.mutate();
              }}
            >
              {mutation.isPending ? "Creating…" : "Create issue"}
            </Button>
          </DialogActions>
        </Modal>
      )}

      {issues.length === 0 ? (
        <Blankslate icon={<CommentIcon size={26} />} title={`No ${state} issues`} />
      ) : (
        <>
        <Box>
          {issues.map((issue, i) => (
            <Link
              key={issue.id}
              to={`/ui/repos/${owner}/${repo}/issues/${issue.number}`}
              className="flex items-start gap-2.5"
              style={{
                padding: "0.7rem 1rem",
                borderBottom: i < issues.length - 1 ? "1px solid var(--color-border)" : "none",
                textDecoration: "none",
              }}
            >
              <span style={{ marginTop: "0.1rem", color: issue.state === "open" ? "var(--gh-open)" : "var(--gh-merged)" }}>
                {issue.state === "open" ? <IssueOpenedIcon /> : <IssueClosedIcon />}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span style={{ fontSize: "0.92rem", fontWeight: 600, color: "var(--color-fg)" }}>
                    {issue.title}
                  </span>
                  <LabelPills labels={issue.labels} />
                </div>
                <div className="mt-1" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                  #{issue.number} opened by {issue.user?.login} ·{" "}
                  {new Date(issue.created_at).toLocaleDateString()}
                  {issue.comments > 0 && ` · ${issue.comments} comments`}
                </div>
              </div>
            </Link>
          ))}
        </Box>
        {hasMore && (
          <div className="mt-3 flex justify-center">
            <Button variant="ghost" size="sm" disabled={isLoadingMore} onClick={loadMore}>
              {isLoadingMore ? "Loading…" : "Load more"}
            </Button>
          </div>
        )}
        </>
      )}
    </div>
  );
}

function IssueDetail({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const counts = useOpenCounts(owner, repo);
  const { data: issue, isLoading, isError, error } = useQuery({
    queryKey: ["issue", owner, repo, number],
    queryFn: () => fetchIssueDetail(owner, repo, number),
  });
  const { data: comments = [] } = useQuery({
    queryKey: ["issue-comments", owner, repo, number],
    queryFn: () => fetchIssueComments(owner, repo, number),
    enabled: !!issue,
  });

  if (isError) {
    if (isNotFound(error)) {
      return (
        <div>
          <RepoHeader owner={owner} repo={repo} active="issues" {...counts} />
          <Blankslate
            icon={<IssueOpenedIcon size={26} />}
            title={`Issue #${number} not found`}
          >
            It may have been deleted, or the number may be wrong.
          </Blankslate>
        </div>
      );
    }
    return <InlineError title={`Failed to load issue #${number}`} detail={String(error)} />;
  }
  if (isLoading || !issue) return <Spinner label={`loading issue #${number}`} />;

  const open = issue.state === "open";
  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="issues" {...counts} />

      <div className="mb-1 flex flex-wrap items-baseline gap-2">
        <h1 style={{ fontSize: "1.4rem", fontWeight: 600, color: "var(--color-fg)" }}>
          {issue.title}{" "}
          <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}>#{issue.number}</span>
        </h1>
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <StateLabel
          state={open ? "open" : "closed"}
          icon={open ? <IssueOpenedIcon size={15} /> : <IssueClosedIcon size={15} />}
        >
          {open ? "Open" : "Closed"}
        </StateLabel>
        <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
          {issue.user?.login} opened this on {new Date(issue.created_at).toLocaleDateString()} ·{" "}
          {comments.length} comment{comments.length === 1 ? "" : "s"}
        </span>
      </div>

      <CommentCard login={issue.user?.login} body={issue.body} date={issue.created_at} isOp />
      <CommentList comments={comments} />
      {comments.length === 0 && (
        <div style={{ padding: "0.5rem 0", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
          No comments yet.
        </div>
      )}
    </div>
  );
}
