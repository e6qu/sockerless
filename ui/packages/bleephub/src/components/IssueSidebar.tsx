import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoLabels,
  fetchRepoMilestones,
  addIssueLabels,
  removeIssueLabel,
  setIssueMilestone,
} from "../api.js";
import { LabelPills } from "./LabelPills.js";
import { ErrorBanner } from "./ui.js";
import { GearIcon } from "./octicons.js";

/** One collapsible-looking sidebar section with a header + gear affordance. */
function SidebarSection({
  title,
  children,
  last,
}: {
  title: string;
  children: ReactNode;
  last?: boolean;
}) {
  return (
    <div
      style={{
        padding: "0.85rem 0",
        borderBottom: last ? "none" : "1px solid var(--color-border)",
      }}
    >
      <div
        className="mb-2 flex items-center justify-between"
        style={{ fontSize: "0.78rem", fontWeight: 600, color: "var(--color-fg-muted)" }}
      >
        <span>{title}</span>
        <GearIcon size={14} style={{ color: "var(--color-fg-subtle)" }} />
      </div>
      {children}
    </div>
  );
}

/**
 * GitHub's issue/PR right sidebar: Assignees, Labels, Projects, Milestone,
 * Development and a participants block. For issues, Labels and Milestone are
 * editable (the same controls the triage panel exposed); for PRs the sidebar
 * additionally renders a Reviewers slot on top. Errors surface, never swallowed.
 */
export function IssueSidebar({
  owner,
  repo,
  number,
  kind,
  assignees,
  labels,
  milestone,
  participants,
  reviewers,
  development,
}: {
  owner: string;
  repo: string;
  number: number;
  kind: "issue" | "pr";
  assignees: string[];
  labels: { name: string; color: string }[];
  milestone: { number: number; title: string; state: string } | null;
  participants: string[];
  reviewers?: ReactNode;
  development?: ReactNode;
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
    qc.invalidateQueries({ queryKey: [kind === "pr" ? "pr" : "issue", owner, repo, number] });
    qc.invalidateQueries({ queryKey: [kind === "pr" ? "prs" : "issues", owner, repo] });
    qc.invalidateQueries({ queryKey: ["pr-timeline", owner, repo, number] });
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
    mutationFn: (m: number | null) => setIssueMilestone(owner, repo, number, m),
    onSuccess: invalidate,
    onError: (err: Error) => setError(err.message),
  });

  const applied = new Set(labels.map((l) => l.name));
  const addable = repoLabels.filter((l) => !applied.has(l.name));
  const muted = { fontSize: "0.82rem", color: "var(--color-fg-muted)" } as const;

  return (
    <aside style={{ width: "100%" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}

      {kind === "pr" && reviewers && <SidebarSection title="Reviewers">{reviewers}</SidebarSection>}

      <SidebarSection title="Assignees">
        {assignees.length === 0 ? (
          <span style={muted}>No one —</span>
        ) : (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            {assignees.map((a) => (
              <span key={a} style={{ fontSize: "0.82rem", fontWeight: 600, color: "var(--color-fg)" }}>
                {a}
              </span>
            ))}
          </div>
        )}
      </SidebarSection>

      <SidebarSection title="Labels">
        {labelsError ? (
          <InlineError inline title="Failed to load repo labels" detail={String(labelsErr)} />
        ) : (
          <div className="flex flex-wrap items-center gap-1.5">
            {labels.length === 0 && <span style={muted}>None yet</span>}
            {labels.map((l) => (
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
                    cursor: "pointer",
                  }}
                >
                  ✕
                </button>
              </span>
            ))}
            {addable.length > 0 && (
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
            )}
          </div>
        )}
      </SidebarSection>

      <SidebarSection title="Projects">
        <span style={muted}>None yet</span>
      </SidebarSection>

      <SidebarSection title="Milestone">
        {milestonesError ? (
          <InlineError inline title="Failed to load milestones" detail={String(milestonesErr)} />
        ) : kind === "issue" ? (
          <select
            aria-label="Set milestone"
            value={milestone?.number ?? ""}
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
        ) : milestone ? (
          <span style={{ fontSize: "0.82rem", color: "var(--color-fg)" }}>{milestone.title}</span>
        ) : (
          <span style={muted}>No milestone</span>
        )}
      </SidebarSection>

      <SidebarSection title="Development">
        {development ?? <span style={muted}>No branches or pull requests</span>}
      </SidebarSection>

      <SidebarSection title={`${participants.length} participant${participants.length === 1 ? "" : "s"}`} last>
        {participants.length === 0 ? (
          <span style={muted}>No participants yet</span>
        ) : (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            {participants.map((p) => (
              <span key={p} style={{ fontSize: "0.82rem", color: "var(--color-fg)" }}>
                {p}
              </span>
            ))}
          </div>
        )}
      </SidebarSection>
    </aside>
  );
}
