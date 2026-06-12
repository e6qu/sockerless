import { useMemo, useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchWorkflowRun,
  fetchWorkflowRunAttempt,
  fetchRunJobs,
  fetchJobLogs,
  fetchRunArtifacts,
  fetchPendingDeployments,
  reviewPendingDeployments,
  cancelRun,
  rerunRun,
  rerunFailedJobs,
  isNotFound,
} from "../api.js";
import type { GithubJob, GithubJobStep, GithubWorkflowRun } from "../types.js";
import { formatDuration } from "../utils/format.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { RunStatusIcon } from "../components/RunStatusIcon.js";
import { Box, Blankslate, Button, ErrorBanner } from "../components/ui.js";
import {
  BranchIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  DownloadIcon,
  PlayIcon,
} from "../components/octicons.js";

/** A run still producing output — drives the ~2s live-tail polling. */
function runIsActive(run: GithubWorkflowRun | undefined): boolean {
  return !!run && run.status !== "completed";
}

export function RunDetailPage() {
  const { owner = "", repo = "", runId = "" } = useParams<{
    owner: string;
    repo: string;
    runId: string;
  }>();
  const counts = useOpenCounts(owner, repo);
  const id = parseInt(runId, 10);

  const runQ = useQuery({
    queryKey: ["run", owner, repo, id],
    queryFn: () => fetchWorkflowRun(owner, repo, id),
    enabled: !!owner && !!repo && Number.isFinite(id),
    refetchInterval: (query) => (runIsActive(query.state.data) ? 2000 : false),
  });
  const run = runQ.data;
  const active = runIsActive(run);

  const jobsQ = useQuery({
    queryKey: ["run-jobs", owner, repo, id],
    queryFn: () => fetchRunJobs(owner, repo, id),
    enabled: !!run,
    refetchInterval: active ? 2000 : false,
  });
  const jobs = jobsQ.data?.items ?? [];

  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const selectedJob = jobs.find((j) => j.id === selectedJobId) ?? jobs[0] ?? null;

  if (runQ.isError) {
    if (isNotFound(runQ.error)) {
      return (
        <div>
          <RepoHeader owner={owner} repo={repo} active="actions" {...counts} />
          <Blankslate icon={<PlayIcon size={26} />} title={`Run #${runId} not found`}>
            It may have been deleted, or the id may be wrong.
          </Blankslate>
        </div>
      );
    }
    return <InlineError title={`Failed to load run #${runId}`} detail={String(runQ.error)} />;
  }
  if (runQ.isLoading || !run) return <Spinner label={`loading run #${runId}`} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="actions" {...counts} />
      <RunHeader owner={owner} repo={repo} run={run} />
      <PendingDeploymentsBanner owner={owner} repo={repo} run={run} />

      <div className="mt-4 flex flex-wrap items-start gap-5 md:flex-nowrap">
        <aside className="w-full shrink-0 md:w-64">
          <Box header={<span style={{ fontWeight: 600, color: "var(--color-fg)" }}>{run.name}</span>}>
            {jobsQ.isLoading && (
              <div style={{ padding: "0.75rem 1rem" }}>
                <Spinner label="loading jobs" />
              </div>
            )}
            {jobsQ.isError && (
              <InlineError inline title="Failed to load jobs" detail={String(jobsQ.error)} />
            )}
            {jobsQ.data && jobs.length === 0 && (
              <div style={{ padding: "0.75rem 1rem", fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
                No jobs recorded for this run.
              </div>
            )}
            {jobs.map((job, i) => (
              <button
                key={job.id}
                type="button"
                onClick={() => setSelectedJobId(job.id)}
                className="flex w-full items-center gap-2 text-left"
                style={{
                  padding: "0.55rem 1rem",
                  fontSize: "0.84rem",
                  fontWeight: selectedJob?.id === job.id ? 600 : 500,
                  color: "var(--color-fg)",
                  background:
                    selectedJob?.id === job.id
                      ? "color-mix(in srgb, var(--color-fg-muted) 10%, transparent)"
                      : "transparent",
                  border: "none",
                  borderBottom: i < jobs.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <RunStatusIcon status={job.status} conclusion={job.conclusion} size={15} />
                <span className="min-w-0 flex-1 truncate">{job.name}</span>
              </button>
            ))}
          </Box>
        </aside>

        <div className="min-w-0 flex-1">
          {selectedJob ? (
            <JobPane owner={owner} repo={repo} job={selectedJob} live={active} />
          ) : (
            jobsQ.data && <Blankslate title="No job selected" />
          )}
          <ArtifactsSection owner={owner} repo={repo} runId={id} />
        </div>
      </div>
    </div>
  );
}

// ─── Run header: title, meta, attempt selector, action buttons ──────────

function RunHeader({
  owner,
  repo,
  run,
}: {
  owner: string;
  repo: string;
  run: GithubWorkflowRun;
}) {
  const qc = useQueryClient();
  const [attempt, setAttempt] = useState<number | null>(null);

  // Attempt history is optional server surface: probe attempt 1 and hide
  // the selector when the endpoint 404s.
  const attemptsSupportedQ = useQuery({
    queryKey: ["run-attempt-probe", owner, repo, run.id],
    queryFn: () => fetchWorkflowRunAttempt(owner, repo, run.id, 1),
    enabled: run.run_attempt > 1,
    retry: false,
  });
  const attemptsSupported = run.run_attempt > 1 && attemptsSupportedQ.isSuccess;

  const attemptQ = useQuery({
    queryKey: ["run-attempt", owner, repo, run.id, attempt],
    queryFn: () => fetchWorkflowRunAttempt(owner, repo, run.id, attempt!),
    enabled: attemptsSupported && attempt !== null && attempt !== run.run_attempt,
  });
  const shown = attempt !== null && attempt !== run.run_attempt && attemptQ.data ? attemptQ.data : run;

  const invalidateRun = () => {
    void qc.invalidateQueries({ queryKey: ["run", owner, repo, run.id] });
    void qc.invalidateQueries({ queryKey: ["run-jobs", owner, repo, run.id] });
    void qc.invalidateQueries({ queryKey: ["runs", owner, repo] });
  };
  const cancelMutation = useMutation({ mutationFn: () => cancelRun(owner, repo, run.id), onSuccess: invalidateRun });
  const rerunMutation = useMutation({ mutationFn: () => rerunRun(owner, repo, run.id), onSuccess: invalidateRun });
  const rerunFailedMutation = useMutation({
    mutationFn: () => rerunFailedJobs(owner, repo, run.id),
    onSuccess: invalidateRun,
  });

  const cancellable = run.status === "queued" || run.status === "in_progress" || run.status === "waiting";
  const completed = run.status === "completed";
  const mutationError =
    cancelMutation.error ?? rerunMutation.error ?? rerunFailedMutation.error;

  return (
    <header className="border-b pb-4" style={{ borderColor: "var(--color-border)" }}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h1
            className="flex items-center gap-2"
            style={{ fontSize: "1.35rem", fontWeight: 600, color: "var(--color-fg)", lineHeight: 1.2 }}
          >
            <RunStatusIcon status={shown.status} conclusion={shown.conclusion} size={20} />
            <span className="min-w-0 break-words">{shown.name}</span>
            <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}>#{shown.run_number}</span>
          </h1>
          <div
            className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1"
            style={{ fontSize: "0.84rem", color: "var(--color-fg-muted)" }}
          >
            <span className="inline-flex items-center gap-1">
              <BranchIcon size={13} />
              <span className="font-mono" style={{ color: "var(--color-accent)" }}>
                {shown.head_branch}
              </span>
            </span>
            <span className="font-mono">{shown.head_sha.slice(0, 7)}</span>
            <span>{shown.event}</span>
            {shown.actor && <span>by {shown.actor.login}</span>}
            <span>{new Date(shown.created_at).toLocaleString()}</span>
            {attemptsSupported && (
              <span className="inline-flex items-center gap-1">
                <label htmlFor="run-attempt-select">Attempt</label>
                <select
                  id="run-attempt-select"
                  value={attempt ?? run.run_attempt}
                  onChange={(e) => setAttempt(parseInt(e.target.value, 10))}
                  style={{
                    padding: "0.1rem 0.3rem",
                    fontSize: "0.8rem",
                    background: "var(--color-bg-subtle)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-sm)",
                    color: "var(--color-fg)",
                  }}
                >
                  {Array.from({ length: run.run_attempt }, (_, i) => i + 1).map((n) => (
                    <option key={n} value={n}>
                      #{n}
                      {n === run.run_attempt ? " (latest)" : ""}
                    </option>
                  ))}
                </select>
              </span>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {cancellable && (
            <Button
              variant="danger"
              size="sm"
              disabled={cancelMutation.isPending}
              onClick={() => cancelMutation.mutate()}
            >
              {cancelMutation.isPending ? "Cancelling…" : "Cancel workflow"}
            </Button>
          )}
          {completed && (
            <Button
              variant="secondary"
              size="sm"
              disabled={rerunMutation.isPending}
              onClick={() => rerunMutation.mutate()}
            >
              {rerunMutation.isPending ? "Re-running…" : "Re-run all jobs"}
            </Button>
          )}
          {completed && run.conclusion === "failure" && (
            <Button
              variant="secondary"
              size="sm"
              disabled={rerunFailedMutation.isPending}
              onClick={() => rerunFailedMutation.mutate()}
            >
              {rerunFailedMutation.isPending ? "Re-running…" : "Re-run failed jobs"}
            </Button>
          )}
        </div>
      </div>
      {attemptQ.isError && (
        <div className="mt-2">
          <ErrorBanner>Failed to load attempt: {String(attemptQ.error)}</ErrorBanner>
        </div>
      )}
      {mutationError != null && (
        <div className="mt-2">
          <ErrorBanner>{String(mutationError)}</ErrorBanner>
        </div>
      )}
    </header>
  );
}

// ─── Pending environment approvals ──────────────────────────────────────

function PendingDeploymentsBanner({
  owner,
  repo,
  run,
}: {
  owner: string;
  repo: string;
  run: GithubWorkflowRun;
}) {
  const qc = useQueryClient();
  const [comment, setComment] = useState("");
  const pendingQ = useQuery({
    queryKey: ["pending-deployments", owner, repo, run.id],
    queryFn: () => fetchPendingDeployments(owner, repo, run.id),
    enabled: run.status === "waiting",
    refetchInterval: run.status === "waiting" ? 2000 : false,
  });

  const review = useMutation({
    mutationFn: (state: "approved" | "rejected") =>
      reviewPendingDeployments(owner, repo, run.id, {
        environment_ids: (pendingQ.data ?? []).map((p) => p.environment.id),
        state,
        comment,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["pending-deployments", owner, repo, run.id] });
      void qc.invalidateQueries({ queryKey: ["run", owner, repo, run.id] });
      void qc.invalidateQueries({ queryKey: ["run-jobs", owner, repo, run.id] });
    },
  });

  if (run.status !== "waiting") return null;
  if (pendingQ.isError) {
    return (
      <div className="mt-4">
        <InlineError title="Failed to load pending deployments" detail={String(pendingQ.error)} />
      </div>
    );
  }
  const pending = pendingQ.data ?? [];
  if (pending.length === 0) return null;

  const envNames = pending.map((p) => p.environment.name);
  const canApprove = pending.some((p) => p.current_user_can_approve);

  return (
    <div
      className="mt-4"
      style={{
        padding: "0.85rem 1rem",
        background: "var(--color-status-warn-soft)",
        border: "1px solid color-mix(in srgb, var(--color-status-warn) 40%, transparent)",
        borderRadius: "var(--radius-md)",
      }}
    >
      <div style={{ fontSize: "0.9rem", fontWeight: 600, color: "var(--color-fg)" }}>
        This workflow is waiting for review to deploy to{" "}
        <span className="font-mono">{envNames.join(", ")}</span>
      </div>
      {canApprove ? (
        <>
          <label
            htmlFor="deployment-review-comment"
            className="mt-2 block"
            style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}
          >
            Leave a comment (optional)
          </label>
          <textarea
            id="deployment-review-comment"
            rows={2}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            className="mt-1 w-full"
            style={{ resize: "vertical", fontSize: "0.84rem" }}
          />
          {review.isError && <ErrorBanner>{String(review.error)}</ErrorBanner>}
          <div className="mt-2 flex gap-2">
            <Button
              variant="primary"
              size="sm"
              disabled={review.isPending}
              onClick={() => review.mutate("approved")}
            >
              Approve and deploy
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={review.isPending}
              onClick={() => review.mutate("rejected")}
            >
              Reject
            </Button>
          </div>
        </>
      ) : (
        <div className="mt-1" style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          You are not authorized to approve this deployment.
        </div>
      )}
    </div>
  );
}

// ─── Job pane: steps with per-step log slices ───────────────────────────

interface LogSegment {
  title: string;
  lines: string[];
}

/**
 * Split a raw job log into ##[group]…##[endgroup] segments. Lines may be
 * prefixed with ISO timestamps, so the markers are matched anywhere in
 * the line. Logs without group markers yield no segments — the caller
 * then falls back to showing the whole log under the job.
 */
function segmentJobLog(text: string): { segments: LogSegment[]; lines: string[] } {
  const lines = text.length === 0 ? [] : text.replace(/\n$/, "").split("\n");
  const segments: LogSegment[] = [];
  let current: LogSegment | null = null;
  for (const line of lines) {
    const group = line.match(/##\[group\](.*)$/);
    if (group) {
      if (current) segments.push(current);
      current = { title: group[1].trim(), lines: [] };
      continue;
    }
    if (/##\[endgroup\]/.test(line)) {
      if (current) segments.push(current);
      current = null;
      continue;
    }
    if (current) current.lines.push(line);
  }
  if (current) segments.push(current);
  return { segments, lines };
}

function JobPane({
  owner,
  repo,
  job,
  live,
}: {
  owner: string;
  repo: string;
  job: GithubJob;
  live: boolean;
}) {
  const logsQ = useQuery({
    queryKey: ["job-logs", owner, repo, job.id],
    queryFn: () => fetchJobLogs(owner, repo, job.id),
    refetchInterval: live ? 2000 : false,
  });
  const { segments, lines } = useMemo(
    () => segmentJobLog(logsQ.data ?? ""),
    [logsQ.data],
  );
  const segmentByStep = useMemo(() => {
    const map = new Map<number, LogSegment>();
    for (const step of job.steps) {
      const seg = segments.find((s) => s.title === step.name);
      if (seg) map.set(step.number, seg);
    }
    return map;
  }, [segments, job.steps]);
  const stepSliced = segmentByStep.size > 0;

  return (
    <Box
      header={
        <div className="flex w-full items-center justify-between gap-2">
          <span className="inline-flex items-center gap-2">
            <RunStatusIcon status={job.status} conclusion={job.conclusion} size={15} />
            <span style={{ fontWeight: 600, color: "var(--color-fg)" }}>{job.name}</span>
          </span>
          <span className="inline-flex items-center gap-2">
            {job.labels.length > 0 && (
              <span className="font-mono" style={{ fontSize: "0.7rem" }}>
                {job.labels.join(", ")}
              </span>
            )}
            <span className="tabular-nums">{formatDuration(job.started_at, job.completed_at)}</span>
          </span>
        </div>
      }
    >
      {job.steps.length === 0 ? (
        <div style={{ padding: "0.75rem 1rem", fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          No steps recorded for this job.
        </div>
      ) : (
        job.steps.map((step) => (
          <StepRow
            key={step.number}
            step={step}
            segment={stepSliced ? segmentByStep.get(step.number) ?? null : null}
            expandable={stepSliced && segmentByStep.has(step.number)}
          />
        ))
      )}

      {logsQ.isError && (
        <InlineError inline title="Failed to load job logs" detail={String(logsQ.error)} />
      )}
      {!stepSliced && lines.length > 0 && (
        <div style={{ borderTop: "1px solid var(--color-border)" }}>
          <div
            style={{
              padding: "0.45rem 1rem",
              fontSize: "0.76rem",
              fontWeight: 600,
              color: "var(--color-fg-muted)",
              background: "var(--color-bg-subtle)",
            }}
          >
            Job log
          </div>
          <LogBlock lines={lines} />
        </div>
      )}
    </Box>
  );
}

function StepRow({
  step,
  segment,
  expandable,
}: {
  step: GithubJobStep;
  segment: LogSegment | null;
  expandable: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ borderBottom: "1px solid var(--color-border)" }}>
      <button
        type="button"
        disabled={!expandable}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={expandable ? open : undefined}
        className="flex w-full items-center gap-2 text-left"
        style={{
          padding: "0.5rem 1rem",
          fontSize: "0.84rem",
          color: "var(--color-fg)",
          background: "transparent",
          border: "none",
          cursor: expandable ? "pointer" : "default",
        }}
      >
        <span style={{ color: "var(--color-fg-subtle)", visibility: expandable ? "visible" : "hidden" }}>
          {open ? <ChevronDownIcon size={12} /> : <ChevronRightIcon size={12} />}
        </span>
        <RunStatusIcon status={step.status} conclusion={step.conclusion} size={14} />
        <span className="min-w-0 flex-1 truncate">{step.name}</span>
        <span className="tabular-nums" style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
          {formatDuration(step.started_at, step.completed_at)}
        </span>
      </button>
      {open && segment && <LogBlock lines={segment.lines} />}
    </div>
  );
}

function LogBlock({ lines }: { lines: string[] }) {
  return (
    <pre
      className="font-mono"
      style={{
        margin: 0,
        padding: "0.6rem 1rem 0.6rem 2.4rem",
        fontSize: "0.74rem",
        lineHeight: 1.6,
        color: "var(--color-fg)",
        background: "var(--color-bg-subtle)",
        overflowX: "auto",
        whiteSpace: "pre-wrap",
        wordBreak: "break-word",
      }}
    >
      {lines.join("\n")}
    </pre>
  );
}

// ─── Artifacts ───────────────────────────────────────────────────────────

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function ArtifactsSection({ owner, repo, runId }: { owner: string; repo: string; runId: number }) {
  const artifactsQ = useQuery({
    queryKey: ["run-artifacts", owner, repo, runId],
    queryFn: () => fetchRunArtifacts(owner, repo, runId),
  });
  if (artifactsQ.isLoading) return null;
  if (artifactsQ.isError) {
    return (
      <div className="mt-5">
        <InlineError title="Failed to load artifacts" detail={String(artifactsQ.error)} />
      </div>
    );
  }
  const artifacts = artifactsQ.data?.items ?? [];
  if (artifacts.length === 0) return null;

  return (
    <div className="mt-5">
      <Box header={<span style={{ fontWeight: 600, color: "var(--color-fg)" }}>Artifacts</span>}>
        {artifacts.map((a, i) => (
          <div
            key={a.id}
            className="flex items-center gap-3"
            style={{
              padding: "0.6rem 1rem",
              borderBottom: i < artifacts.length - 1 ? "1px solid var(--color-border)" : "none",
            }}
          >
            <div className="min-w-0 flex-1">
              <span style={{ fontSize: "0.86rem", fontWeight: 500, color: "var(--color-fg)" }}>
                {a.name}
              </span>
              {a.expired && (
                <span className="ml-2" style={{ fontSize: "0.74rem", color: "var(--color-fg-subtle)" }}>
                  expired
                </span>
              )}
            </div>
            <span className="tabular-nums" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
              {formatBytes(a.size_in_bytes)}
            </span>
            {!a.expired && (
              // The zip endpoint replies with a 302 to the download —
              // a plain anchor follows it; fetch()+blob would not stream.
              <a
                href={`/api/v3/repos/${owner}/${repo}/actions/artifacts/${a.id}/zip`}
                className="inline-flex items-center gap-1"
                style={{ fontSize: "0.78rem", color: "var(--color-accent)", textDecoration: "none" }}
                download
              >
                <DownloadIcon size={13} /> Download
              </a>
            )}
          </div>
        ))}
      </Box>
    </div>
  );
}

