import { useMemo, useState, type ReactNode } from "react";
import { useParams, Link, useNavigate } from "react-router";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoPRsFilteredPage,
  fetchPRDetail,
  fetchPRCommits,
  fetchCheckRuns,
  mergePR,
  isNotFound,
  fetchAuthenticatedUser,
  fetchPRReviews,
  createPRReview,
  dismissPRReview,
  fetchPRReviewComments,
  replyToPRReviewComment,
  fetchPRReviewThreads,
  setPRReviewThreadResolved,
  fetchPRRequestedReviewers,
  requestPRReviewers,
  removePRRequestedReviewers,
  fetchCombinedStatus,
  fetchIssueTimeline,
  fetchIssueReactions,
  addIssueReaction,
  removeIssueReaction,
  fetchIssueCommentReactions,
  addIssueCommentReaction,
  removeIssueCommentReaction,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import type {
  GithubCheckRun,
  GithubCommitStatus,
  GithubCommitStatusState,
  GithubPR,
  GithubPRReview,
  GithubPRReviewComment,
  GithubReviewState,
  GithubTimelineItem,
  ListFilterState,
} from "../types.js";
import { formatDuration } from "../utils/format.js";
import { CommentCard } from "../components/CommentCard.js";
import { RepoHeader } from "../components/Shell.js";
import { RunStatusIcon } from "../components/RunStatusIcon.js";
import { ReactionBar } from "../components/ReactionBar.js";
import { IssueSidebar } from "../components/IssueSidebar.js";
import { PRFilesView } from "../components/PRFilesView.js";
import {
  ListControls,
  filterAndSortItems,
  emptyFilters,
  type ListItemAccessors,
} from "../components/ListControls.js";
import { Button, Box, Blankslate, StateLabel, SectionLabel, FormLabel, Tabs } from "../components/ui.js";
import {
  PullRequestIcon,
  MergedIcon,
  PullClosedIcon,
  BranchIcon,
  CheckCircleIcon,
  XCircleIcon,
  DotFillIcon,
} from "../components/octicons.js";

const prAccessors: ListItemAccessors<GithubPR> = {
  labels: (p) => p.labels,
  author: (p) => p.user?.login ?? null,
  assignees: () => [],
  milestone: () => null,
  comments: () => 0,
  createdAt: (p) => p.created_at,
  updatedAt: (p) => p.updated_at,
};

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

function usePRClosedCount(owner: string, repo: string): number | string | undefined {
  const { data } = useQuery({
    queryKey: ["prs", owner, repo, "closed", "count"],
    queryFn: () => fetchRepoPRsFilteredPage(owner, repo, { state: "closed" }),
    enabled: !!owner && !!repo,
  });
  if (!data) return undefined;
  return data.nextUrl ? `${data.items.length}+` : data.items.length;
}

function PRList({ owner, repo }: { owner: string; repo: string }) {
  const [state, setState] = useState<"open" | "closed">("open");
  const [filters, setFilters] = useState<ListFilterState>(emptyFilters);
  const counts = useOpenCounts(owner, repo);
  const closedCount = usePRClosedCount(owner, repo);

  const query = useInfiniteQuery({
    queryKey: ["prs", owner, repo, state, "paged"],
    queryFn: ({ pageParam }) => fetchRepoPRsFilteredPage(owner, repo, { state }, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextUrl ?? undefined,
    enabled: !!owner && !!repo,
  });
  const rawPRs = useMemo(() => query.data?.pages.flatMap((p) => p.items) ?? [], [query.data]);
  const prs = useMemo(() => filterAndSortItems(rawPRs, filters, prAccessors), [rawPRs, filters]);

  if (query.isLoading) return <Spinner label="loading pull requests" />;
  if (query.isError)
    return <InlineError title="Failed to load pull requests" detail={String(query.error)} />;

  const hasMore = query.hasNextPage;
  const isLoadingMore = query.isFetchingNextPage;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="pulls" {...counts} />

      <ListControls
        kind="pr"
        state={state}
        onState={setState}
        openCount={counts.prCount}
        closedCount={closedCount}
        items={rawPRs}
        filters={filters}
        onFilters={setFilters}
        accessors={prAccessors}
      />

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
            <Button variant="ghost" size="sm" disabled={isLoadingMore} onClick={() => query.fetchNextPage()}>
              {isLoadingMore ? "Loading…" : "Load more"}
            </Button>
          </div>
        )}
        </>
      )}
    </div>
  );
}

type PRTab = "conversation" | "commits" | "files" | "checks";

function PRDetail({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const counts = useOpenCounts(owner, repo);
  const [tab, setTab] = useState<PRTab>("conversation");
  const { data: pr, isLoading, isError, error } = useQuery({
    queryKey: ["pr", owner, repo, number],
    queryFn: () => fetchPRDetail(owner, repo, number),
  });
  const viewerQ = useQuery({ queryKey: ["viewer"], queryFn: fetchAuthenticatedUser });
  const viewerLogin = typeof viewerQ.data?.login === "string" ? viewerQ.data.login : null;

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

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="pulls" {...counts} />

      <h1 className="mb-2" style={{ fontSize: "1.5rem", fontWeight: 400, color: "var(--color-fg)" }}>
        {pr.title} <span style={{ color: "var(--color-fg-muted)" }}>#{pr.number}</span>
      </h1>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <StateLabel state={s} icon={<PRStateIcon pr={pr} size={15} />}>
          {stateLabel}
        </StateLabel>
        <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
          <strong style={{ color: "var(--color-fg)" }}>{pr.user?.login}</strong> wants to merge into{" "}
          <span className="font-mono" style={{ color: "var(--color-accent)" }}>{pr.base.ref}</span>
          {" from "}
          <span className="font-mono" style={{ color: "var(--color-accent)" }}>{pr.head.ref}</span>
        </span>
      </div>

      <Tabs
        active={tab}
        onChange={(k) => setTab(k as PRTab)}
        items={[
          { key: "conversation", label: "Conversation" },
          { key: "commits", label: "Commits" },
          { key: "files", label: "Files changed" },
          { key: "checks", label: "Checks" },
        ]}
      />

      {tab === "conversation" && (
        <div className="flex flex-col gap-6 lg:flex-row">
          <div className="min-w-0 flex-1">
            {viewerQ.isError && (
              <InlineError inline title="Failed to load current user" detail={String(viewerQ.error)} />
            )}
            <CommentCard login={pr.user?.login} body={pr.body} date={pr.created_at} isOp />
            <ReactionBar
              queryKey={["pr-body-reactions", owner, repo, number]}
              fetchList={() => fetchIssueReactions(owner, repo, number)}
              add={(content) => addIssueReaction(owner, repo, number, content)}
              remove={(reactionId) => removeIssueReaction(owner, repo, number, reactionId)}
              viewerLogin={viewerLogin}
            />
            <ConversationTimeline owner={owner} repo={repo} number={number} viewerLogin={viewerLogin} />
            <ReviewThreadsSection owner={owner} repo={repo} number={number} />
            <ReviewsSection owner={owner} repo={repo} number={number} />
            <MergeBox owner={owner} repo={repo} number={number} pr={pr} />
          </div>
          <div style={{ width: "100%", maxWidth: "16rem", flexShrink: 0 }}>
            <IssueSidebar
              owner={owner}
              repo={repo}
              number={number}
              kind="pr"
              assignees={[]}
              labels={pr.labels}
              milestone={null}
              participants={pr.user?.login ? [pr.user.login] : []}
              reviewers={<RequestedReviewersSection owner={owner} repo={repo} number={number} />}
              development={
                <span style={{ fontSize: "0.82rem", color: "var(--color-fg)" }}>
                  <span className="font-mono" style={{ color: "var(--color-accent)" }}>
                    {pr.head.ref}
                  </span>
                  {" → "}
                  <span className="font-mono">{pr.base.ref}</span>
                </span>
              }
            />
          </div>
        </div>
      )}

      {tab === "commits" && <PRCommitsTab owner={owner} repo={repo} number={number} />}
      {tab === "files" && <PRFilesView owner={owner} repo={repo} number={number} />}
      {tab === "checks" && <ChecksSection owner={owner} repo={repo} sha={pr.head.sha} standalone />}
    </div>
  );
}

// ─── Merge box (bottom of Conversation) ──────────────────────────────────

const MERGE_METHODS: { value: "merge" | "squash" | "rebase"; label: string }[] = [
  { value: "merge", label: "Create a merge commit" },
  { value: "squash", label: "Squash and merge" },
  { value: "rebase", label: "Rebase and merge" },
];

function MergeBox({
  owner,
  repo,
  number,
  pr,
}: {
  owner: string;
  repo: string;
  number: number;
  pr: GithubPR;
}) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [method, setMethod] = useState<"merge" | "squash" | "rebase">("merge");
  const checksQ = useQuery({
    queryKey: ["check-runs", owner, repo, pr.head.sha],
    queryFn: () => fetchCheckRuns(owner, repo, pr.head.sha),
    enabled: !!pr.head.sha,
  });
  const statusQ = useQuery({
    queryKey: ["combined-status", owner, repo, pr.head.sha],
    queryFn: () => fetchCombinedStatus(owner, repo, pr.head.sha),
    enabled: !!pr.head.sha,
  });
  const mergeMutation = useMutation({
    mutationFn: () => mergePR(owner, repo, number, method),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["prs", owner, repo] });
      qc.invalidateQueries({ queryKey: ["pr", owner, repo, number] });
      navigate(`/ui/repos/${owner}/${repo}/pulls`);
    },
  });

  const s = prState(pr);
  if (s === "merged") {
    return (
      <div className="mt-4">
        <Box>
          <div className="flex items-center gap-2" style={{ padding: "0.85rem 1rem" }}>
            <MergedIcon size={18} style={{ color: "var(--gh-merged)" }} />
            <span style={{ fontWeight: 600, color: "var(--gh-merged)" }}>
              Pull request successfully merged and closed
            </span>
          </div>
        </Box>
      </div>
    );
  }
  if (s === "closed") {
    return (
      <div className="mt-4">
        <Box>
          <div className="flex items-center gap-2" style={{ padding: "0.85rem 1rem" }}>
            <PullClosedIcon size={18} style={{ color: "var(--gh-closed)" }} />
            <span style={{ fontWeight: 600, color: "var(--color-fg)" }}>
              This pull request is closed
            </span>
          </div>
        </Box>
      </div>
    );
  }

  const checks = checksQ.data?.items ?? [];
  const statuses = statusQ.data?.statuses ?? [];
  const summary = mergeBoxSummary(checks, statuses);
  const mergeBlocked = pr.mergeable_state === "blocked" || pr.draft;

  return (
    <div className="mt-4">
      <Box>
        <div style={{ padding: "0.85rem 1rem" }}>
          {(checks.length > 0 || statuses.length > 0) && (
            <div className="mb-2 flex items-center gap-2" style={{ color: summary.color, fontWeight: 600 }}>
              {summary.pending ? (
                <DotFillIcon size={16} />
              ) : summary.color === "var(--gh-open)" ? (
                <CheckCircleIcon size={16} />
              ) : (
                <XCircleIcon size={16} />
              )}
              {summary.label}
            </div>
          )}
          <div
            className="mb-2 flex items-center gap-2"
            style={{ fontSize: "0.86rem", color: pr.draft ? "var(--color-fg-muted)" : "var(--gh-open)" }}
          >
            {pr.draft ? (
              <>
                <PullRequestIcon size={16} /> This pull request is still a work in progress
              </>
            ) : mergeBlocked ? (
              <span style={{ color: "var(--color-status-error)" }}>
                Merging is blocked — required checks must pass
              </span>
            ) : (
              <>
                <CheckCircleIcon size={16} /> This branch has no conflicts with the base branch
              </>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="primary"
              size="sm"
              disabled={mergeMutation.isPending || mergeBlocked}
              onClick={() => mergeMutation.mutate()}
            >
              {mergeMutation.isPending
                ? "Merging…"
                : method === "squash"
                  ? "Squash and merge"
                  : method === "rebase"
                    ? "Rebase and merge"
                    : "Merge pull request"}
            </Button>
            <select
              aria-label="Merge method"
              value={method}
              onChange={(e) => setMethod(e.target.value as "merge" | "squash" | "rebase")}
              disabled={mergeMutation.isPending}
              style={{ fontSize: "0.82rem" }}
            >
              {MERGE_METHODS.map((m) => (
                <option key={m.value} value={m.value}>
                  {m.label}
                </option>
              ))}
            </select>
          </div>
          {mergeMutation.isError && (
            <div className="mt-2" style={{ fontSize: "0.8rem", color: "var(--color-status-error)" }}>
              Merge failed:{" "}
              {mergeMutation.error instanceof Error ? mergeMutation.error.message : "unknown error"}
            </div>
          )}
        </div>
      </Box>
    </div>
  );
}

// ─── Commits tab ─────────────────────────────────────────────────────────

function PRCommitsTab({ owner, repo, number }: { owner: string; repo: string; number: number }) {
  const q = useQuery({
    queryKey: ["pr-commits", owner, repo, number],
    queryFn: () => fetchPRCommits(owner, repo, number),
  });
  if (q.isLoading) return <Spinner label="loading commits" />;
  if (q.isError) return <InlineError title="Failed to load commits" detail={String(q.error)} />;
  const commits = q.data ?? [];
  if (commits.length === 0) {
    return <Blankslate icon={<BranchIcon size={26} />} title="No commits" />;
  }
  return (
    <Box>
      {commits.map((c, i) => (
        <div
          key={c.sha}
          className="flex items-start gap-3"
          style={{
            padding: "0.7rem 1rem",
            borderBottom: i < commits.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <BranchIcon size={14} style={{ marginTop: "0.2rem", color: "var(--color-fg-muted)" }} />
          <div className="min-w-0 flex-1">
            <div style={{ fontSize: "0.88rem", fontWeight: 600, color: "var(--color-fg)" }}>
              {c.commit.message.split("\n")[0]}
            </div>
            <div className="mt-0.5" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
              {c.commit.author.name} committed on{" "}
              {new Date(c.commit.author.date).toLocaleDateString()}
            </div>
          </div>
          <span
            className="font-mono tabular-nums"
            style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}
          >
            {c.sha.slice(0, 7)}
          </span>
        </div>
      ))}
    </Box>
  );
}

// ─── Merge-box checks + commit statuses summary ──────────────────────────

function mergeBoxSummary(
  checks: GithubCheckRun[],
  statuses: GithubCommitStatus[],
): { label: string; color: string; pending: boolean } {
  const checkFailed = checks.some(
    (c) =>
      c.status === "completed" &&
      c.conclusion !== null &&
      !["success", "neutral", "skipped"].includes(c.conclusion),
  );
  const statusFailed = statuses.some((st) => st.state === "failure" || st.state === "error");
  if (checkFailed || statusFailed) {
    return { label: "Some checks were not successful", color: "var(--color-status-error)", pending: false };
  }
  const pending =
    checks.some((c) => c.status !== "completed") || statuses.some((st) => st.state === "pending");
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

function CommitStatusIcon({ state }: { state: GithubCommitStatusState }) {
  if (state === "success") {
    return <CheckCircleIcon size={15} style={{ color: "var(--gh-open)" }} />;
  }
  if (state === "failure" || state === "error") {
    return <XCircleIcon size={15} style={{ color: "var(--color-status-error)" }} />;
  }
  return <DotFillIcon size={15} style={{ color: "var(--color-status-warn)" }} />;
}

function ChecksSection({
  owner,
  repo,
  sha,
  standalone,
}: {
  owner: string;
  repo: string;
  sha: string;
  standalone?: boolean;
}) {
  const checksQ = useQuery({
    queryKey: ["check-runs", owner, repo, sha],
    queryFn: () => fetchCheckRuns(owner, repo, sha),
    enabled: !!sha,
    refetchInterval: (query) =>
      query.state.data?.items.some((c) => c.status !== "completed") ? 5000 : false,
  });
  const statusQ = useQuery({
    queryKey: ["combined-status", owner, repo, sha],
    queryFn: () => fetchCombinedStatus(owner, repo, sha),
    enabled: !!sha,
    refetchInterval: (query) =>
      query.state.data?.statuses.some((st) => st.state === "pending") ? 5000 : false,
  });

  if (checksQ.isLoading || statusQ.isLoading) {
    return standalone ? <Spinner label="loading checks" /> : null;
  }
  if (checksQ.isError) {
    return <InlineError title="Failed to load checks" detail={String(checksQ.error)} />;
  }
  const checks = checksQ.data?.items ?? [];
  const statuses = statusQ.data?.statuses ?? [];
  // GitHub hides the checks box entirely for commits with neither check
  // runs nor commit statuses.
  if (statusQ.isError && checks.length === 0) {
    return <InlineError title="Failed to load commit statuses" detail={String(statusQ.error)} />;
  }
  if (checks.length === 0 && statuses.length === 0) {
    return standalone ? (
      <Blankslate icon={<CheckCircleIcon size={26} />} title="No checks reported for this commit" />
    ) : null;
  }

  const summary = mergeBoxSummary(checks, statuses);
  const rowStyle = (last: boolean) =>
    ({
      padding: "0.55rem 1rem",
      borderBottom: last ? "none" : "1px solid var(--color-border)",
      textDecoration: "none",
    }) as const;

  return (
    <div className="mb-4">
      {statusQ.isError && (
        <InlineError inline title="Failed to load commit statuses" detail={String(statusQ.error)} />
      )}
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
        {statuses.map((status, i) => {
          const last = i === statuses.length - 1 && checks.length === 0;
          const row = (
            <>
              <CommitStatusIcon state={status.state} />
              <span className="min-w-0 flex-1 truncate" style={{ fontSize: "0.86rem", color: "var(--color-fg)" }}>
                {status.context}
                {status.description && (
                  <span style={{ color: "var(--color-fg-muted)" }}> — {status.description}</span>
                )}
              </span>
            </>
          );
          return status.target_url ? (
            <a key={status.context} href={status.target_url} className="flex items-center gap-2" style={rowStyle(last)}>
              {row}
            </a>
          ) : (
            <div key={status.context} className="flex items-center gap-2" style={rowStyle(last)}>
              {row}
            </div>
          );
        })}
        {checks.map((check, i) => {
          const last = i === checks.length - 1;
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
          return runLink ? (
            <Link key={check.id} to={runLink} className="flex items-center gap-2" style={rowStyle(last)}>
              {row}
            </Link>
          ) : check.details_url ? (
            <a key={check.id} href={check.details_url} className="flex items-center gap-2" style={rowStyle(last)}>
              {row}
            </a>
          ) : (
            <div key={check.id} className="flex items-center gap-2" style={rowStyle(last)}>
              {row}
            </div>
          );
        })}
      </Box>
    </div>
  );
}

// ─── Requested reviewers ─────────────────────────────────────────────────

function RequestedReviewersSection({
  owner,
  repo,
  number,
}: {
  owner: string;
  repo: string;
  number: number;
}) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["pr-requested-reviewers", owner, repo, number],
    queryFn: () => fetchPRRequestedReviewers(owner, repo, number),
  });
  const [login, setLogin] = useState("");
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: ["pr-requested-reviewers", owner, repo, number] });
  const add = useMutation({
    mutationFn: (l: string) => requestPRReviewers(owner, repo, number, [l]),
    onSuccess: () => {
      setLogin("");
      invalidate();
    },
  });
  const remove = useMutation({
    mutationFn: (l: string) => removePRRequestedReviewers(owner, repo, number, [l]),
    onSuccess: invalidate,
  });

  if (q.isLoading) return null;

  // Rendered inside the sidebar's "Reviewers" section, which supplies the
  // heading — so this body carries none of its own.
  return (
    <div>
      {q.isError || !q.data ? (
        <InlineError inline title="Failed to load requested reviewers" detail={String(q.error)} />
      ) : (
        <div className="flex flex-wrap items-center gap-2">
          {q.data.users.length === 0 && q.data.teams.length === 0 && (
            <span style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
              No reviewers requested.
            </span>
          )}
          {q.data.users.map((u) => (
            <span
              key={u.login}
              className="inline-flex items-center gap-1.5"
              style={{
                border: "1px solid var(--color-border)",
                borderRadius: "2rem",
                padding: "0.15rem 0.3rem 0.15rem 0.6rem",
                fontSize: "0.8rem",
                color: "var(--color-fg)",
              }}
            >
              {u.login}
              <button
                type="button"
                aria-label={`remove reviewer ${u.login}`}
                disabled={remove.isPending}
                onClick={() => remove.mutate(u.login)}
                style={{
                  border: "none",
                  background: "transparent",
                  cursor: "pointer",
                  color: "var(--color-fg-muted)",
                  fontSize: "0.85rem",
                  lineHeight: 1,
                  padding: "0.1rem 0.3rem",
                }}
              >
                ✕
              </button>
            </span>
          ))}
          {q.data.teams.map((t) => (
            <span
              key={t.slug}
              style={{
                border: "1px solid var(--color-border)",
                borderRadius: "2rem",
                padding: "0.15rem 0.6rem",
                fontSize: "0.8rem",
                color: "var(--color-fg)",
              }}
            >
              {t.name}
            </span>
          ))}
          <span className="inline-flex items-center gap-1.5">
            <input
              aria-label="reviewer login"
              value={login}
              onChange={(e) => setLogin(e.target.value)}
              placeholder="username"
              style={{ fontSize: "0.8rem", padding: "0.2rem 0.5rem", maxWidth: "11rem" }}
            />
            <Button
              size="sm"
              disabled={!login.trim() || add.isPending}
              onClick={() => add.mutate(login.trim())}
            >
              {add.isPending ? "Requesting…" : "Request review"}
            </Button>
          </span>
        </div>
      )}
      {add.isError && (
        <InlineError inline title="Failed to request reviewer" detail={String(add.error)} />
      )}
      {remove.isError && (
        <InlineError inline title="Failed to remove reviewer" detail={String(remove.error)} />
      )}
    </div>
  );
}

// ─── Conversation timeline ───────────────────────────────────────────────

function reviewStateText(state: string): string {
  switch (state) {
    case "APPROVED":
      return "approved these changes";
    case "CHANGES_REQUESTED":
      return "requested changes";
    case "DISMISSED":
      return "reviewed (dismissed)";
    default:
      return "reviewed";
  }
}

function TimelineEventRow({ item }: { item: GithubTimelineItem }) {
  const actor = item.actor?.login ?? item.user?.login;
  const when = item.created_at ?? item.submitted_at ?? null;

  let text: ReactNode;
  switch (item.event) {
    case "reviewed":
      text = <>{reviewStateText(item.state ?? "")}</>;
      break;
    case "labeled":
    case "unlabeled":
      text = (
        <>
          {item.event === "labeled" ? "added the" : "removed the"}{" "}
          {item.label ? (
            <span
              className="font-mono"
              style={{
                border: `1px solid #${item.label.color}`,
                borderRadius: "2rem",
                padding: "0 0.45rem",
                fontSize: "0.74rem",
              }}
            >
              {item.label.name}
            </span>
          ) : (
            "unknown"
          )}{" "}
          label
        </>
      );
      break;
    case "assigned":
    case "unassigned":
      text = (
        <>
          {item.event === "assigned" ? "assigned" : "unassigned"}{" "}
          <strong>{item.assignee?.login ?? "unknown"}</strong>
        </>
      );
      break;
    case "renamed":
      text = (
        <>
          changed the title from <em>{item.rename?.from}</em> to <em>{item.rename?.to}</em>
        </>
      );
      break;
    default:
      // Render unrecognised events honestly by their wire name.
      text = <>{item.event.replaceAll("_", " ")}</>;
  }

  return (
    <div
      className="flex items-start gap-2"
      style={{ padding: "0.35rem 0.25rem", fontSize: "0.82rem", color: "var(--color-fg-muted)" }}
    >
      <DotFillIcon size={14} style={{ marginTop: "0.15rem", color: "var(--color-fg-subtle)" }} />
      <span className="min-w-0 flex-1">
        <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{actor}</span> {text}
        {when && <span> · {new Date(when).toLocaleString()}</span>}
      </span>
    </div>
  );
}

function ConversationTimeline({
  owner,
  repo,
  number,
  viewerLogin,
}: {
  owner: string;
  repo: string;
  number: number;
  viewerLogin: string | null;
}) {
  const q = useQuery({
    queryKey: ["pr-timeline", owner, repo, number],
    queryFn: () => fetchIssueTimeline(owner, repo, number),
  });

  if (q.isLoading) return null;
  if (q.isError) {
    return <InlineError title="Failed to load conversation" detail={String(q.error)} />;
  }
  const items = q.data ?? [];

  return (
    <>
      {items.map((item, i) => {
        if (item.event === "commented" && typeof item.id === "number") {
          return (
            <div key={`commented-${item.id}`}>
              <CommentCard
                login={item.user?.login}
                body={item.body}
                date={item.created_at ?? ""}
              />
              <ReactionBar
                queryKey={["issue-comment-reactions", owner, repo, item.id]}
                fetchList={() => fetchIssueCommentReactions(owner, repo, item.id as number)}
                add={(content) => addIssueCommentReaction(owner, repo, item.id as number, content)}
                remove={(reactionId) =>
                  removeIssueCommentReaction(owner, repo, item.id as number, reactionId)
                }
                viewerLogin={viewerLogin}
              />
            </div>
          );
        }
        if (item.event === "reviewed" && item.body) {
          return (
            <div key={`reviewed-${item.id ?? i}`}>
              <TimelineEventRow item={item} />
              <div style={{ marginLeft: "1.4rem" }}>
                <CommentCard
                  login={item.user?.login}
                  body={item.body}
                  date={item.submitted_at ?? item.created_at ?? ""}
                />
              </div>
            </div>
          );
        }
        return <TimelineEventRow key={`${item.event}-${item.id ?? i}`} item={item} />;
      })}
    </>
  );
}

// ─── Review comment threads ──────────────────────────────────────────────

interface ReviewThreadGroup {
  root: GithubPRReviewComment;
  comments: GithubPRReviewComment[];
}

/** Group flat review comments into threads by following in_reply_to_id. */
function groupReviewThreads(comments: GithubPRReviewComment[]): ReviewThreadGroup[] {
  const byId = new Map(comments.map((c) => [c.id, c]));
  const rootOf = (c: GithubPRReviewComment): GithubPRReviewComment => {
    let cur = c;
    const seen = new Set<number>();
    while (cur.in_reply_to_id != null && byId.has(cur.in_reply_to_id) && !seen.has(cur.id)) {
      seen.add(cur.id);
      cur = byId.get(cur.in_reply_to_id) as GithubPRReviewComment;
    }
    return cur;
  };
  const groups = new Map<number, ReviewThreadGroup>();
  const sorted = [...comments].sort(
    (a, b) => a.created_at.localeCompare(b.created_at) || a.id - b.id,
  );
  for (const c of sorted) {
    const root = rootOf(c);
    const g = groups.get(root.id) ?? { root, comments: [] };
    g.comments.push(c);
    groups.set(root.id, g);
  }
  return [...groups.values()];
}

function ReviewThreadCard({
  owner,
  repo,
  number,
  group,
  threadInfo,
}: {
  owner: string;
  repo: string;
  number: number;
  group: ReviewThreadGroup;
  threadInfo: { id: number; isResolved: boolean } | null;
}) {
  const qc = useQueryClient();
  const [replyBody, setReplyBody] = useState("");
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["pr-review-comments", owner, repo, number] });
    qc.invalidateQueries({ queryKey: ["pr-review-threads", owner, repo, number] });
  };
  const reply = useMutation({
    mutationFn: () => replyToPRReviewComment(owner, repo, number, group.root.id, replyBody.trim()),
    onSuccess: () => {
      setReplyBody("");
      invalidate();
    },
  });
  const resolve = useMutation({
    mutationFn: (resolved: boolean) => {
      if (!threadInfo) {
        throw new Error("thread resolution state unavailable");
      }
      return setPRReviewThreadResolved(owner, repo, number, threadInfo.id, resolved);
    },
    onSuccess: invalidate,
  });

  const resolved = threadInfo?.isResolved ?? false;

  return (
    <div className="mb-3">
      <Box
        header={
          <span className="flex items-center gap-2" style={{ fontSize: "0.82rem" }}>
            <span className="font-mono min-w-0 flex-1 truncate" style={{ color: "var(--color-fg)" }}>
              {group.root.path}
              {group.root.line != null && `:${group.root.line}`}
            </span>
            {resolved && (
              <span
                style={{
                  border: "1px solid var(--color-border)",
                  borderRadius: "2rem",
                  padding: "0.05rem 0.5rem",
                  fontSize: "0.72rem",
                  color: "var(--color-fg-muted)",
                }}
              >
                Resolved
              </span>
            )}
            {threadInfo && (
              <Button
                size="sm"
                variant="ghost"
                disabled={resolve.isPending}
                onClick={() => resolve.mutate(!resolved)}
              >
                {resolve.isPending ? "…" : resolved ? "Unresolve" : "Resolve"}
              </Button>
            )}
          </span>
        }
      >
        {group.root.diff_hunk && (
          <pre
            className="font-mono"
            style={{
              margin: 0,
              padding: "0.6rem 1rem",
              fontSize: "0.76rem",
              lineHeight: 1.5,
              overflowX: "auto",
              background: "var(--color-bg-subtle)",
              borderBottom: "1px solid var(--color-border)",
              color: "var(--color-fg)",
            }}
          >
            {group.root.diff_hunk}
          </pre>
        )}
        {group.comments.map((c) => (
          <div
            key={c.id}
            style={{
              padding: "0.6rem 1rem",
              borderBottom: "1px solid var(--color-border)",
              fontSize: "0.86rem",
            }}
          >
            <div style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)", marginBottom: "0.25rem" }}>
              <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{c.user?.login}</span>{" "}
              commented {new Date(c.created_at).toLocaleString()}
            </div>
            <div style={{ whiteSpace: "pre-wrap", wordBreak: "break-word", color: "var(--color-fg)" }}>
              {c.body}
            </div>
          </div>
        ))}
        <div className="flex items-center gap-2" style={{ padding: "0.55rem 1rem" }}>
          <input
            aria-label={`reply to thread on ${group.root.path}`}
            value={replyBody}
            onChange={(e) => setReplyBody(e.target.value)}
            placeholder="Reply…"
            className="min-w-0 flex-1"
            style={{ fontSize: "0.82rem", padding: "0.25rem 0.5rem" }}
          />
          <Button
            size="sm"
            disabled={!replyBody.trim() || reply.isPending}
            onClick={() => reply.mutate()}
          >
            {reply.isPending ? "Replying…" : "Reply"}
          </Button>
        </div>
      </Box>
      {reply.isError && (
        <InlineError inline title="Failed to reply" detail={String(reply.error)} />
      )}
      {resolve.isError && (
        <InlineError inline title="Failed to update thread resolution" detail={String(resolve.error)} />
      )}
    </div>
  );
}

function ReviewThreadsSection({
  owner,
  repo,
  number,
}: {
  owner: string;
  repo: string;
  number: number;
}) {
  const commentsQ = useQuery({
    queryKey: ["pr-review-comments", owner, repo, number],
    queryFn: () => fetchPRReviewComments(owner, repo, number),
  });
  const threadsQ = useQuery({
    queryKey: ["pr-review-threads", owner, repo, number],
    queryFn: () => fetchPRReviewThreads(owner, repo, number),
  });

  if (commentsQ.isLoading) return null;
  if (commentsQ.isError) {
    return <InlineError title="Failed to load review comments" detail={String(commentsQ.error)} />;
  }
  const comments = commentsQ.data ?? [];
  if (comments.length === 0) return null;

  const threadInfoByCommentId = new Map<number, { id: number; isResolved: boolean }>();
  for (const t of threadsQ.data ?? []) {
    for (const c of t.comments) {
      threadInfoByCommentId.set(c.id, { id: t.id, isResolved: t.isResolved });
    }
  }
  const groups = groupReviewThreads(comments);

  return (
    <div className="mb-4">
      <SectionLabel>Review comments</SectionLabel>
      {threadsQ.isError && (
        <InlineError
          inline
          title="Failed to load thread resolution state"
          detail={String(threadsQ.error)}
        />
      )}
      {groups.map((g) => (
        <ReviewThreadCard
          key={g.root.id}
          owner={owner}
          repo={repo}
          number={number}
          group={g}
          threadInfo={threadInfoByCommentId.get(g.root.id) ?? null}
        />
      ))}
    </div>
  );
}

// ─── Reviews ─────────────────────────────────────────────────────────────

const reviewBadge: Record<GithubReviewState, { label: string; color: string }> = {
  APPROVED: { label: "Approved", color: "var(--gh-open)" },
  CHANGES_REQUESTED: { label: "Changes requested", color: "var(--color-status-error)" },
  COMMENTED: { label: "Commented", color: "var(--color-fg-muted)" },
  DISMISSED: { label: "Dismissed", color: "var(--color-fg-muted)" },
  PENDING: { label: "Pending", color: "var(--color-status-warn)" },
};

function ReviewStateBadge({ state }: { state: GithubReviewState }) {
  const badge = reviewBadge[state];
  return (
    <span
      style={{
        border: `1px solid ${badge.color}`,
        color: badge.color,
        borderRadius: "2rem",
        padding: "0.05rem 0.55rem",
        fontSize: "0.74rem",
        fontWeight: 600,
      }}
    >
      {badge.label}
    </span>
  );
}

function ReviewRow({
  owner,
  repo,
  number,
  review,
}: {
  owner: string;
  repo: string;
  number: number;
  review: GithubPRReview;
}) {
  const qc = useQueryClient();
  const [dismissing, setDismissing] = useState(false);
  const [message, setMessage] = useState("");
  const dismiss = useMutation({
    mutationFn: () => dismissPRReview(owner, repo, number, review.id, message.trim()),
    onSuccess: () => {
      setDismissing(false);
      setMessage("");
      qc.invalidateQueries({ queryKey: ["pr-reviews", owner, repo, number] });
      qc.invalidateQueries({ queryKey: ["pr-timeline", owner, repo, number] });
    },
  });
  const dismissable = review.state === "APPROVED" || review.state === "CHANGES_REQUESTED";

  return (
    <div
      style={{
        padding: "0.6rem 1rem",
        borderBottom: "1px solid var(--color-border)",
        fontSize: "0.86rem",
      }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{review.user?.login}</span>
        <ReviewStateBadge state={review.state} />
        {review.submitted_at && (
          <span style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
            {new Date(review.submitted_at).toLocaleString()}
          </span>
        )}
        <span className="flex-1" />
        {dismissable && !dismissing && (
          <Button size="sm" variant="ghost" onClick={() => setDismissing(true)}>
            Dismiss
          </Button>
        )}
      </div>
      {review.body && (
        <div
          style={{
            marginTop: "0.3rem",
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            color: "var(--color-fg)",
          }}
        >
          {review.body}
        </div>
      )}
      {dismissing && (
        <div className="mt-2 flex items-center gap-2">
          <input
            aria-label={`dismissal message for review ${review.id}`}
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="Reason for dismissing…"
            className="min-w-0 flex-1"
            style={{ fontSize: "0.82rem", padding: "0.25rem 0.5rem" }}
          />
          <Button
            size="sm"
            variant="danger"
            disabled={!message.trim() || dismiss.isPending}
            onClick={() => dismiss.mutate()}
          >
            {dismiss.isPending ? "Dismissing…" : "Confirm dismiss"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setDismissing(false)}>
            Cancel
          </Button>
        </div>
      )}
      {dismiss.isError && (
        <InlineError inline title="Failed to dismiss review" detail={String(dismiss.error)} />
      )}
    </div>
  );
}

function ReviewsSection({
  owner,
  repo,
  number,
}: {
  owner: string;
  repo: string;
  number: number;
}) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["pr-reviews", owner, repo, number],
    queryFn: () => fetchPRReviews(owner, repo, number),
  });
  const [body, setBody] = useState("");
  const submit = useMutation({
    mutationFn: (event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT") =>
      createPRReview(owner, repo, number, { body: body.trim(), event }),
    onSuccess: () => {
      setBody("");
      qc.invalidateQueries({ queryKey: ["pr-reviews", owner, repo, number] });
      qc.invalidateQueries({ queryKey: ["pr-timeline", owner, repo, number] });
    },
  });

  if (q.isLoading) return null;

  return (
    <div className="mb-4">
      <SectionLabel>Reviews</SectionLabel>
      {q.isError || !q.data ? (
        <InlineError title="Failed to load reviews" detail={String(q.error)} />
      ) : q.data.length === 0 ? (
        <div className="mb-3" style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          No reviews yet.
        </div>
      ) : (
        <div className="mb-3">
          <Box>
            {q.data.map((review) => (
              <ReviewRow
                key={review.id}
                owner={owner}
                repo={repo}
                number={number}
                review={review}
              />
            ))}
          </Box>
        </div>
      )}

      <Box header={<span style={{ fontWeight: 600 }}>Submit a review</span>}>
        <div style={{ padding: "0.7rem 1rem" }}>
          <FormLabel id="review-body">Review comment</FormLabel>
          <textarea
            id="review-body"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={3}
            placeholder="Leave a review comment…"
            className="mb-2 w-full"
            style={{ resize: "vertical" }}
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="primary"
              disabled={submit.isPending}
              onClick={() => submit.mutate("APPROVE")}
            >
              Approve
            </Button>
            <Button
              size="sm"
              variant="danger"
              disabled={!body.trim() || submit.isPending}
              onClick={() => submit.mutate("REQUEST_CHANGES")}
            >
              Request changes
            </Button>
            <Button
              size="sm"
              disabled={!body.trim() || submit.isPending}
              onClick={() => submit.mutate("COMMENT")}
            >
              Comment
            </Button>
            {submit.isPending && (
              <span style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>Submitting…</span>
            )}
          </div>
          {submit.isError && (
            <InlineError inline title="Failed to submit review" detail={String(submit.error)} />
          )}
        </div>
      </Box>
    </div>
  );
}
