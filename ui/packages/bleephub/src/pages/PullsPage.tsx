import { useParams, Link, useNavigate } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoPRsPage,
  fetchPRDetail,
  fetchIssueComments,
  fetchCheckRuns,
  mergePR,
  isNotFound,
} from "../api.js";
import { useRepoItemList } from "../hooks/useRepoItemList.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import type { GithubCheckRun, GithubPR } from "../types.js";
import { formatDuration } from "../utils/format.js";
import { CommentCard, CommentList } from "../components/CommentCard.js";
import { StateToggle } from "../components/StateToggle.js";
import { RepoHeader } from "../components/Shell.js";
import { RunStatusIcon } from "../components/RunStatusIcon.js";
import { Button, Box, Blankslate, StateLabel } from "../components/ui.js";
import { PullRequestIcon, MergedIcon, PullClosedIcon, BranchIcon } from "../components/octicons.js";

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

function prState(pr: GithubPR): "open" | "merged" | "closed" | "draft" {
  if (pr.merged) return "merged";
  if (pr.state === "open") return pr.draft ? "draft" : "open";
  return "closed";
}

function PRStateIcon({ pr, size }: { pr: GithubPR; size?: number }) {
  const s = prState(pr);
  if (s === "merged") return <MergedIcon size={size} style={{ color: "var(--gh-merged)" }} />;
  if (s === "closed") return <PullClosedIcon size={size} style={{ color: "var(--gh-closed)" }} />;
  if (s === "draft") return <PullRequestIcon size={size} style={{ color: "var(--gh-draft)" }} />;
  return <PullRequestIcon size={size} style={{ color: "var(--gh-open)" }} />;
}

function PRList({ owner, repo }: { owner: string; repo: string }) {
  const {
    state,
    setState,
    items: prs,
    isLoading,
    isError,
    error,
    hasMore,
    loadMore,
    isLoadingMore,
  } = useRepoItemList("prs", owner, repo, fetchRepoPRsPage);
  const counts = useOpenCounts(owner, repo);

  if (isLoading) return <Spinner label="loading pull requests" />;
  if (isError) return <InlineError title="Failed to load pull requests" detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="pulls" {...counts} />

      <div className="mb-4">
        <StateToggle
          value={state}
          options={["open", "closed"] as const}
          labels={{ open: "Open", closed: "Closed / Merged" }}
          icons={{ open: <PullRequestIcon size={14} />, closed: <MergedIcon size={14} /> }}
          onChange={setState}
        />
      </div>

      {prs.length === 0 ? (
        <Blankslate icon={<PullRequestIcon size={26} />} title={`No ${state} pull requests`} />
      ) : (
        <>
        <Box>
          {prs.map((pr, i) => (
            <Link
              key={pr.id}
              to={`/ui/repos/${owner}/${repo}/pulls/${pr.number}`}
              className="flex items-start gap-2.5"
              style={{
                padding: "0.7rem 1rem",
                borderBottom: i < prs.length - 1 ? "1px solid var(--color-border)" : "none",
                textDecoration: "none",
              }}
            >
              <span style={{ marginTop: "0.1rem" }}>
                <PRStateIcon pr={pr} />
              </span>
              <div className="min-w-0 flex-1">
                <div style={{ fontSize: "0.92rem", fontWeight: 600, color: "var(--color-fg)" }}>
                  {pr.title}
                  {pr.draft && (
                    <span style={{ marginLeft: "0.5rem", fontSize: "0.74rem", color: "var(--color-fg-subtle)", fontWeight: 400 }}>
                      Draft
                    </span>
                  )}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-x-2" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                  <span>#{pr.number}</span>
                  <span className="inline-flex items-center gap-1">
                    <BranchIcon size={12} />
                    <span className="font-mono" style={{ color: "var(--color-accent)" }}>{pr.head.ref}</span>
                    {" → "}
                    <span className="font-mono">{pr.base.ref}</span>
                  </span>
                  <span>· opened by {pr.user?.login} · {new Date(pr.created_at).toLocaleDateString()}</span>
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

function PRDetail({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const counts = useOpenCounts(owner, repo);
  const { data: pr, isLoading, isError, error } = useQuery({
    queryKey: ["pr", owner, repo, number],
    queryFn: () => fetchPRDetail(owner, repo, number),
  });
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

  if (isError) {
    if (isNotFound(error)) {
      return (
        <div>
          <RepoHeader owner={owner} repo={repo} active="pulls" {...counts} />
          <Blankslate
            icon={<PullRequestIcon size={26} />}
            title={`Pull request #${number} not found`}
          >
            It may have been deleted, or the number may be wrong.
          </Blankslate>
        </div>
      );
    }
    return <InlineError title={`Failed to load PR #${number}`} detail={String(error)} />;
  }
  if (isLoading || !pr) return <Spinner label={`loading PR #${number}`} />;

  const s = prState(pr);
  const stateLabel = s === "merged" ? "Merged" : s === "closed" ? "Closed" : s === "draft" ? "Draft" : "Open";
  const isMergeable = pr.state === "open" && !pr.draft && pr.merged_at === null;
  const mergeBlocked = pr.mergeable_state === "blocked";

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="pulls" {...counts} />

      <h1 className="mb-2" style={{ fontSize: "1.4rem", fontWeight: 600, color: "var(--color-fg)" }}>
        {pr.title} <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}>#{pr.number}</span>
      </h1>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <StateLabel state={s} icon={<PRStateIcon pr={pr} size={15} />}>
            {stateLabel}
          </StateLabel>
          <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
            <span className="font-mono" style={{ color: "var(--color-accent)" }}>{pr.head.ref}</span>
            {" → "}
            <span className="font-mono">{pr.base.ref}</span> · opened by {pr.user?.login}
          </span>
        </div>
        {isMergeable && (
          <div className="flex items-center gap-2">
            {mergeBlocked && (
              <span style={{ fontSize: "0.8rem", color: "var(--color-status-error)" }}>
                Merging is blocked — required checks must pass
              </span>
            )}
            <Button
              variant="primary"
              size="sm"
              disabled={mergeMutation.isPending || mergeBlocked}
              onClick={() => mergeMutation.mutate()}
            >
              {mergeMutation.isPending ? "Merging…" : "Merge pull request"}
            </Button>
          </div>
        )}
      </div>

      <ChecksSection owner={owner} repo={repo} sha={pr.head.sha} />

      <CommentCard login={pr.user?.login} body={pr.body} date={pr.created_at} isOp />
      <CommentList comments={comments} />
    </div>
  );
}

// ─── Merge-box checks summary ────────────────────────────────────────────

function checksSummary(checks: GithubCheckRun[]): {
  label: string;
  color: string;
  pending: boolean;
} {
  const failed = checks.some(
    (c) =>
      c.status === "completed" &&
      c.conclusion !== null &&
      !["success", "neutral", "skipped"].includes(c.conclusion),
  );
  if (failed) {
    return { label: "Some checks were not successful", color: "var(--color-status-error)", pending: false };
  }
  const pending = checks.some((c) => c.status !== "completed");
  if (pending) {
    return { label: "Some checks haven't completed yet", color: "var(--color-status-warn)", pending: true };
  }
  return { label: "All checks have passed", color: "var(--gh-open)", pending: false };
}

/** Turn a check's details_url into an in-app run link when it points at a run. */
function runLinkFor(owner: string, repo: string, detailsUrl: string): string | null {
  const m = detailsUrl.match(/\/actions\/runs\/(\d+)/);
  if (!m) return null;
  return `/ui/repos/${owner}/${repo}/actions/runs/${m[1]}`;
}

function ChecksSection({ owner, repo, sha }: { owner: string; repo: string; sha: string }) {
  const checksQ = useQuery({
    queryKey: ["check-runs", owner, repo, sha],
    queryFn: () => fetchCheckRuns(owner, repo, sha),
    enabled: !!sha,
    refetchInterval: (query) =>
      query.state.data?.items.some((c) => c.status !== "completed") ? 5000 : false,
  });

  if (checksQ.isLoading) return null;
  if (checksQ.isError) {
    return <InlineError title="Failed to load checks" detail={String(checksQ.error)} />;
  }
  const checks = checksQ.data?.items ?? [];
  // GitHub hides the checks box entirely for commits with no check runs.
  if (checks.length === 0) return null;

  const summary = checksSummary(checks);

  return (
    <div className="mb-4">
      <Box
        header={
          <span className="inline-flex items-center gap-2" style={{ color: summary.color, fontWeight: 600 }}>
            {summary.pending && (
              <span
                aria-hidden
                className="animate-spin inline-block"
                style={{
                  width: 12,
                  height: 12,
                  border: "2px solid var(--color-status-warn)",
                  borderTopColor: "transparent",
                  borderRadius: "999px",
                }}
              />
            )}
            {summary.label}
          </span>
        }
      >
        {checks.map((check, i) => {
          const runLink = runLinkFor(owner, repo, check.details_url);
          const row = (
            <>
              <RunStatusIcon status={check.status} conclusion={check.conclusion} size={15} />
              <span className="min-w-0 flex-1 truncate" style={{ fontSize: "0.86rem", color: "var(--color-fg)" }}>
                {check.name}
              </span>
              <span className="tabular-nums" style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
                {formatDuration(check.started_at, check.completed_at)}
              </span>
            </>
          );
          const rowStyle = {
            padding: "0.55rem 1rem",
            borderBottom: i < checks.length - 1 ? "1px solid var(--color-border)" : "none",
            textDecoration: "none",
          } as const;
          return runLink ? (
            <Link key={check.id} to={runLink} className="flex items-center gap-2" style={rowStyle}>
              {row}
            </Link>
          ) : check.details_url ? (
            <a key={check.id} href={check.details_url} className="flex items-center gap-2" style={rowStyle}>
              {row}
            </a>
          ) : (
            <div key={check.id} className="flex items-center gap-2" style={rowStyle}>
              {row}
            </div>
          );
        })}
      </Box>
    </div>
  );
}
