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
  fetchRepoLabels,
  createRepoLabel,
  updateRepoLabel,
  deleteRepoLabel,
  fetchRepoMilestones,
  createRepoMilestone,
  updateRepoMilestone,
  deleteRepoMilestone,
  addIssueLabels,
  removeIssueLabel,
  setIssueMilestone,
} from "../api.js";
import { useRepoItemList } from "../hooks/useRepoItemList.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import type { GithubIssue, GithubLabel, GithubMilestone } from "../types.js";
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
  SectionLabel,
} from "../components/ui.js";
import { IssueOpenedIcon, IssueClosedIcon, CommentIcon, TagIcon } from "../components/octicons.js";

export function IssuesPage({ view }: { view?: "labels" | "milestones" }) {
  const { owner = "", repo = "", number } = useParams<{
    owner: string;
    repo: string;
    number?: string;
  }>();

  if (view === "labels") {
    return <LabelsView owner={owner} repo={repo} />;
  }
  if (view === "milestones") {
    return <MilestonesView owner={owner} repo={repo} />;
  }
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
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={() => navigate(`/ui/repos/${owner}/${repo}/labels`)}>
            <TagIcon size={14} /> Labels
          </Button>
          <Button size="sm" onClick={() => navigate(`/ui/repos/${owner}/${repo}/milestones`)}>
            Milestones
          </Button>
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            New issue
          </Button>
        </div>
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
  const { data: comments = [], isError: commentsError, error: commentsErr } = useQuery({
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

      <IssueTriagePanel owner={owner} repo={repo} number={number} issue={issue} />

      <CommentCard login={issue.user?.login} body={issue.body} date={issue.created_at} isOp />
      {commentsError ? (
        <InlineError inline title="Failed to load comments" detail={String(commentsErr)} />
      ) : (
        <>
          <CommentList comments={comments} />
          {comments.length === 0 && (
            <div style={{ padding: "0.5rem 0", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No comments yet.
            </div>
          )}
        </>
      )}
    </div>
  );
}

/** Label + milestone triage controls on the issue detail view. */
function IssueTriagePanel({
  owner,
  repo,
  number,
  issue,
}: {
  owner: string;
  repo: string;
  number: number;
  issue: GithubIssue;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const { data: repoLabels = [], isError: labelsError, error: labelsErr } = useQuery({
    queryKey: ["labels", owner, repo],
    queryFn: () => fetchRepoLabels(owner, repo),
  });
  const { data: milestones = [], isError: milestonesError, error: milestonesErr } = useQuery({
    queryKey: ["milestones", owner, repo, "all"],
    queryFn: () => fetchRepoMilestones(owner, repo, "all"),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: ["issue", owner, repo, number] });
    qc.invalidateQueries({ queryKey: ["issues", owner, repo] });
  };
  const addLabelMut = useMutation({
    mutationFn: (name: string) => addIssueLabels(owner, repo, number, [name]),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });
  const removeLabelMut = useMutation({
    mutationFn: (name: string) => removeIssueLabel(owner, repo, number, name),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });
  const milestoneMut = useMutation({
    mutationFn: (milestone: number | null) => setIssueMilestone(owner, repo, number, milestone),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  const applied = new Set(issue.labels.map((l) => l.name));
  const addable = repoLabels.filter((l) => !applied.has(l.name));

  return (
    <Box className="mb-4">
      <div style={{ padding: "0.75rem 1rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        <div className="mb-3">
          <div className="mb-1.5" style={{ fontSize: "0.8rem", fontWeight: 600, color: "var(--color-fg-muted)" }}>
            Labels
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            {issue.labels.length === 0 && (
              <span style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>None yet</span>
            )}
            {issue.labels.map((l) => (
              <span key={l.name} className="inline-flex items-center gap-1">
                <LabelPills labels={[l]} />
                <button
                  type="button"
                  aria-label={`Remove label ${l.name}`}
                  onClick={() => removeLabelMut.mutate(l.name)}
                  disabled={removeLabelMut.isPending}
                  style={{
                    border: "none",
                    background: "transparent",
                    color: "var(--color-fg-muted)",
                    fontSize: "0.75rem",
                    lineHeight: 1,
                    padding: "0 0.15rem",
                  }}
                >
                  ✕
                </button>
              </span>
            ))}
            {labelsError ? (
              <InlineError inline title="Failed to load repo labels" detail={String(labelsErr)} />
            ) : (
              addable.length > 0 && (
                <select
                  aria-label="Add label"
                  value=""
                  onChange={(e) => {
                    if (e.target.value) addLabelMut.mutate(e.target.value);
                  }}
                  disabled={addLabelMut.isPending}
                  style={{ fontSize: "0.8rem" }}
                >
                  <option value="">Add label…</option>
                  {addable.map((l) => (
                    <option key={l.id} value={l.name}>
                      {l.name}
                    </option>
                  ))}
                </select>
              )
            )}
          </div>
        </div>
        <div>
          <div className="mb-1.5" style={{ fontSize: "0.8rem", fontWeight: 600, color: "var(--color-fg-muted)" }}>
            Milestone
          </div>
          {milestonesError ? (
            <InlineError inline title="Failed to load milestones" detail={String(milestonesErr)} />
          ) : (
            <select
              aria-label="Set milestone"
              value={issue.milestone?.number ?? ""}
              onChange={(e) =>
                milestoneMut.mutate(e.target.value === "" ? null : parseInt(e.target.value, 10))
              }
              disabled={milestoneMut.isPending}
              style={{ fontSize: "0.8rem" }}
            >
              <option value="">No milestone</option>
              {milestones.map((ms) => (
                <option key={ms.id} value={ms.number}>
                  {ms.title}
                  {ms.state === "closed" ? " (closed)" : ""}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
    </Box>
  );
}

// ─── Repo labels management ─────────────────────────────────────────────

function LabelsView({ owner, repo }: { owner: string; repo: string }) {
  const counts = useOpenCounts(owner, repo);
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<GithubLabel | null>(null);

  const { data: labels, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["labels", owner, repo],
    queryFn: () => fetchRepoLabels(owner, repo),
  });

  const deleteMut = useMutation({
    mutationFn: (name: string) => deleteRepoLabel(owner, repo, name),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["labels", owner, repo] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (isLoading || !labels) {
    if (isError) return <InlineError title="Failed to load labels" detail={String(loadErr)} />;
    return <Spinner label="loading labels" />;
  }

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="issues" {...counts} />
      <div className="mb-4 flex items-center justify-between gap-3">
        <SectionLabel>Labels</SectionLabel>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New label
        </Button>
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {labels.length === 0 ? (
        <Blankslate icon={<TagIcon size={26} />} title="No labels yet">
          Labels help categorize issues and pull requests.
        </Blankslate>
      ) : (
        <Box>
          {labels.map((label, i) => (
            <div
              key={label.id}
              className="flex flex-wrap items-center gap-3"
              style={{
                padding: "0.7rem 1rem",
                borderBottom: i < labels.length - 1 ? "1px solid var(--color-border)" : "none",
              }}
            >
              <LabelPills labels={[label]} />
              <span className="min-w-0 flex-1" style={{ fontSize: "0.83rem", color: "var(--color-fg-muted)" }}>
                {label.description || "No description"}
              </span>
              <Button size="sm" variant="ghost" onClick={() => setEditing(label)}>
                edit
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={deleteMut.isPending}
                onClick={() => {
                  if (confirm(`Delete label ${label.name}?`)) deleteMut.mutate(label.name);
                }}
              >
                delete
              </Button>
            </div>
          ))}
        </Box>
      )}
      {creating && <LabelDialog owner={owner} repo={repo} onClose={() => setCreating(false)} />}
      {editing && (
        <LabelDialog owner={owner} repo={repo} label={editing} onClose={() => setEditing(null)} />
      )}
    </div>
  );
}

function LabelDialog({
  owner,
  repo,
  label,
  onClose,
}: {
  owner: string;
  repo: string;
  label?: GithubLabel;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(label?.name ?? "");
  const [color, setColor] = useState(label?.color ?? "ededed");
  const [description, setDescription] = useState(label?.description ?? "");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      label
        ? updateRepoLabel(owner, repo, label.name, {
            new_name: name.trim(),
            color,
            description,
          })
        : createRepoLabel(owner, repo, { name: name.trim(), color, description }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["labels", owner, repo] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={label ? `Edit label ${label.name}` : "New label"} onClose={onClose}>
      <FormLabel id="label-name">Name</FormLabel>
      <input
        id="label-name"
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="mb-3 w-full"
      />
      <FormLabel id="label-color">Color (hex, without #)</FormLabel>
      <input
        id="label-color"
        value={color}
        onChange={(e) => setColor(e.target.value.replace(/^#/, ""))}
        className="mb-3 w-full"
      />
      <FormLabel id="label-desc">Description (optional)</FormLabel>
      <input
        id="label-desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
      />
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={mutation.isPending}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!name.trim() || mutation.isPending}
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
        >
          {mutation.isPending ? "Saving…" : label ? "Save" : "Create label"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

// ─── Repo milestones management ─────────────────────────────────────────

function MilestonesView({ owner, repo }: { owner: string; repo: string }) {
  const counts = useOpenCounts(owner, repo);
  const qc = useQueryClient();
  const [state, setState] = useState<"open" | "closed">("open");
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const { data: milestones, isLoading, isError, error: loadErr } = useQuery({
    queryKey: ["milestones", owner, repo, state],
    queryFn: () => fetchRepoMilestones(owner, repo, state),
  });

  const invalidate = () => {
    setError(null);
    qc.invalidateQueries({ queryKey: ["milestones", owner, repo] });
  };
  const stateMut = useMutation({
    mutationFn: ({ number, next }: { number: number; next: "open" | "closed" }) =>
      updateRepoMilestone(owner, repo, number, { state: next }),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });
  const deleteMut = useMutation({
    mutationFn: (number: number) => deleteRepoMilestone(owner, repo, number),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  if (isLoading || !milestones) {
    if (isError) return <InlineError title="Failed to load milestones" detail={String(loadErr)} />;
    return <Spinner label="loading milestones" />;
  }

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="issues" {...counts} />
      <div className="mb-4 flex items-center justify-between gap-3">
        <StateToggle
          value={state}
          options={["open", "closed"] as const}
          labels={{ open: "Open", closed: "Closed" }}
          onChange={setState}
        />
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New milestone
        </Button>
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {milestones.length === 0 ? (
        <Blankslate icon={<IssueOpenedIcon size={26} />} title={`No ${state} milestones`}>
          Milestones group issues and pull requests toward a target.
        </Blankslate>
      ) : (
        <Box>
          {milestones.map((ms, i) => (
            <MilestoneRow
              key={ms.id}
              milestone={ms}
              last={i === milestones.length - 1}
              onToggleState={(next) => stateMut.mutate({ number: ms.number, next })}
              onDelete={() => {
                if (confirm(`Delete milestone ${ms.title}?`)) deleteMut.mutate(ms.number);
              }}
              busy={stateMut.isPending || deleteMut.isPending}
            />
          ))}
        </Box>
      )}
      {creating && (
        <MilestoneDialog owner={owner} repo={repo} onClose={() => setCreating(false)} />
      )}
    </div>
  );
}

function MilestoneRow({
  milestone: ms,
  last,
  onToggleState,
  onDelete,
  busy,
}: {
  milestone: GithubMilestone;
  last: boolean;
  onToggleState: (next: "open" | "closed") => void;
  onDelete: () => void;
  busy: boolean;
}) {
  const total = ms.open_issues + ms.closed_issues;
  const pct = total > 0 ? Math.round((ms.closed_issues / total) * 100) : 0;
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      style={{
        padding: "0.7rem 1rem",
        borderBottom: last ? "none" : "1px solid var(--color-border)",
      }}
    >
      <div className="min-w-0 flex-1">
        <div style={{ fontWeight: 600, fontSize: "0.92rem" }}>{ms.title}</div>
        <div className="mt-0.5" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
          {ms.due_on ? `Due ${new Date(ms.due_on).toLocaleDateString()} · ` : ""}
          {pct}% complete · {ms.open_issues} open · {ms.closed_issues} closed
          {ms.description && ` · ${ms.description}`}
        </div>
      </div>
      <Button
        size="sm"
        variant="ghost"
        disabled={busy}
        onClick={() => onToggleState(ms.state === "open" ? "closed" : "open")}
      >
        {ms.state === "open" ? "close" : "reopen"}
      </Button>
      <Button size="sm" variant="danger" disabled={busy} onClick={onDelete}>
        delete
      </Button>
    </div>
  );
}

function MilestoneDialog({
  owner,
  repo,
  onClose,
}: {
  owner: string;
  repo: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [dueOn, setDueOn] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      createRepoMilestone(owner, repo, {
        title: title.trim(),
        description: description || undefined,
        due_on: dueOn ? new Date(dueOn).toISOString() : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["milestones", owner, repo] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title="New milestone" onClose={onClose}>
      <FormLabel id="ms-title">Title</FormLabel>
      <input
        id="ms-title"
        autoFocus
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        className="mb-3 w-full"
      />
      <FormLabel id="ms-desc">Description (optional)</FormLabel>
      <input
        id="ms-desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-3 w-full"
      />
      <FormLabel id="ms-due">Due date (optional)</FormLabel>
      <input
        id="ms-due"
        type="date"
        value={dueOn}
        onChange={(e) => setDueOn(e.target.value)}
        className="mb-4 w-full"
      />
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={mutation.isPending}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!title.trim() || mutation.isPending}
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
        >
          {mutation.isPending ? "Creating…" : "Create milestone"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
