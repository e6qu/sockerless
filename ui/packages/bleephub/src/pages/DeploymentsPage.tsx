import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  createDeploymentStatus,
  fetchDeploymentStatuses,
  fetchDeploymentsPage,
  fetchEnvBranchPolicies,
  fetchEnvProtectionRules,
  fetchEnvironmentsDetail,
  fetchPendingDeployments,
  fetchWorkflowRunsPage,
  reviewPendingDeployments,
} from "../api.js";
import type {
  GithubDeployment,
  GithubDeploymentState,
  GithubDeploymentStatus,
  GithubEnvironmentDetail,
  GithubWorkflowRun,
} from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { PageTitle, Box, Blankslate, Button, Tabs, ErrorBanner, FormLabel } from "../components/ui.js";
import { ChevronDownIcon, ChevronRightIcon } from "../components/octicons.js";

type DeploymentsTab = "deployments" | "environments" | "approvals";

const STATE_COLORS: Record<GithubDeploymentState, string> = {
  success: "var(--gh-open)",
  error: "var(--color-danger-fg)",
  failure: "var(--color-danger-fg)",
  inactive: "var(--color-fg-subtle)",
  in_progress: "var(--color-status-warn)",
  queued: "var(--color-status-warn)",
  pending: "var(--color-status-warn)",
};

const CREATABLE_STATES: GithubDeploymentState[] = [
  "error",
  "failure",
  "inactive",
  "in_progress",
  "queued",
  "pending",
  "success",
];

export function DeploymentsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [tab, setTab] = useState<DeploymentsTab>("deployments");

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="code" />
      <PageTitle title="Deployments" />
      <Tabs
        items={[
          { key: "deployments" as const, label: "Deployments" },
          { key: "environments" as const, label: "Environments" },
          { key: "approvals" as const, label: "Pending approvals" },
        ]}
        active={tab}
        onChange={setTab}
      />
      {tab === "deployments" && <DeploymentsTab owner={owner} repo={repo} />}
      {tab === "environments" && <EnvironmentsTab owner={owner} repo={repo} />}
      {tab === "approvals" && <ApprovalsTab owner={owner} repo={repo} />}
    </div>
  );
}

// ─── Deployments list + statuses timeline ────────────────────────────────

function DeploymentsTab({ owner, repo }: { owner: string; repo: string }) {
  const [extra, setExtra] = useState<GithubDeployment[]>([]);
  const [nextUrl, setNextUrl] = useState<string | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);

  const firstPage = useQuery({
    queryKey: ["deployments", owner, repo],
    queryFn: () => fetchDeploymentsPage(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (firstPage.isLoading) return <Spinner label="loading deployments" />;
  if (firstPage.isError)
    return <InlineError title="Failed to load deployments" detail={String(firstPage.error)} />;

  const deployments = [...(firstPage.data?.items ?? []), ...extra];
  const followUrl = nextUrl ?? firstPage.data?.nextUrl ?? null;

  if (deployments.length === 0)
    return <Blankslate title="No deployments">Deployments created via POST /deployments appear here.</Blankslate>;

  const loadMore = async () => {
    if (!followUrl) return;
    try {
      const page = await fetchDeploymentsPage(owner, repo, followUrl);
      setExtra((prev) => [...prev, ...page.items]);
      setNextUrl(page.nextUrl);
      setPageError(null);
    } catch (err) {
      setPageError(String(err));
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      {pageError && <ErrorBanner>{pageError}</ErrorBanner>}
      <Box>
        {deployments.map((d, i) => (
          <DeploymentRow
            key={d.id}
            owner={owner}
            repo={repo}
            deployment={d}
            last={i === deployments.length - 1}
          />
        ))}
      </Box>
      {followUrl && (
        <div className="flex justify-center">
          <Button variant="secondary" size="sm" onClick={() => void loadMore()}>
            Load more
          </Button>
        </div>
      )}
    </div>
  );
}

function DeploymentRow({
  owner,
  repo,
  deployment,
  last,
}: {
  owner: string;
  repo: string;
  deployment: GithubDeployment;
  last: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ borderBottom: last ? "none" : "1px solid var(--color-border)" }}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 text-left"
        style={{ padding: "0.7rem 1rem", background: "transparent", border: "none" }}
      >
        {open ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
        <div className="min-w-0 flex-1">
          <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
            {deployment.environment}{" "}
            <span style={{ color: "var(--color-fg-subtle)", fontWeight: 400 }}>
              #{deployment.id}
            </span>
            {deployment.production_environment && (
              <span
                className="ml-2"
                style={{
                  fontSize: "0.7rem",
                  color: "var(--color-fg-muted)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "999px",
                  padding: "0.05rem 0.45rem",
                }}
              >
                production
              </span>
            )}
            {deployment.transient_environment && (
              <span
                className="ml-2"
                style={{
                  fontSize: "0.7rem",
                  color: "var(--color-fg-muted)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "999px",
                  padding: "0.05rem 0.45rem",
                }}
              >
                transient
              </span>
            )}
          </div>
          <div className="font-mono" style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
            {deployment.ref} · {deployment.task}
            {deployment.creator ? ` · by ${deployment.creator.login}` : ""} ·{" "}
            {new Date(deployment.created_at).toLocaleString()}
          </div>
        </div>
      </button>
      {open && <DeploymentStatuses owner={owner} repo={repo} deploymentId={deployment.id} />}
    </div>
  );
}

function DeploymentStatuses({
  owner,
  repo,
  deploymentId,
}: {
  owner: string;
  repo: string;
  deploymentId: number;
}) {
  const qc = useQueryClient();
  const [state, setState] = useState<GithubDeploymentState>("success");
  const [description, setDescription] = useState("");
  const [environmentUrl, setEnvironmentUrl] = useState("");

  const statusesQ = useQuery({
    queryKey: ["deployment-statuses", owner, repo, deploymentId],
    queryFn: () => fetchDeploymentStatuses(owner, repo, deploymentId),
  });

  const createMut = useMutation({
    mutationFn: () =>
      createDeploymentStatus(owner, repo, deploymentId, {
        state,
        description: description.trim() || undefined,
        environment_url: environmentUrl.trim() || undefined,
      }),
    onSuccess: () => {
      setDescription("");
      setEnvironmentUrl("");
      void qc.invalidateQueries({ queryKey: ["deployment-statuses", owner, repo, deploymentId] });
    },
  });

  if (statusesQ.isLoading) return <Spinner label="loading statuses" />;
  if (statusesQ.isError)
    return (
      <div style={{ padding: "0 1rem 0.75rem 2.4rem" }}>
        <InlineError title="Failed to load deployment statuses" detail={String(statusesQ.error)} />
      </div>
    );

  const statuses = statusesQ.data ?? [];

  return (
    <div style={{ padding: "0 1rem 0.9rem 2.4rem" }}>
      {statuses.length === 0 ? (
        <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>No statuses yet.</div>
      ) : (
        <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {statuses.map((s) => (
            <StatusEntry key={s.id} status={s} />
          ))}
        </ul>
      )}
      <form
        className="mt-3 flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          createMut.mutate();
        }}
      >
        <label style={{ display: "flex", flexDirection: "column", gap: "0.2rem", fontSize: "0.75rem" }}>
          State
          <select
            aria-label="New status state"
            value={state}
            onChange={(e) => setState(e.target.value as GithubDeploymentState)}
            style={{ fontSize: "0.82rem", padding: "0.3rem 0.4rem" }}
          >
            {CREATABLE_STATES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <label style={{ display: "flex", flexDirection: "column", gap: "0.2rem", fontSize: "0.75rem" }}>
          Description
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="optional"
            style={{ fontSize: "0.82rem", padding: "0.3rem 0.4rem" }}
          />
        </label>
        <label style={{ display: "flex", flexDirection: "column", gap: "0.2rem", fontSize: "0.75rem" }}>
          Environment URL
          <input
            type="text"
            value={environmentUrl}
            onChange={(e) => setEnvironmentUrl(e.target.value)}
            placeholder="optional"
            style={{ fontSize: "0.82rem", padding: "0.3rem 0.4rem" }}
          />
        </label>
        <Button type="submit" variant="secondary" size="sm" disabled={createMut.isPending}>
          Create status
        </Button>
      </form>
      {createMut.isError && <ErrorBanner>{String(createMut.error)}</ErrorBanner>}
    </div>
  );
}

function StatusEntry({ status }: { status: GithubDeploymentStatus }) {
  return (
    <li className="flex items-center gap-2" style={{ padding: "0.3rem 0" }}>
      <span
        aria-hidden
        style={{
          width: 8,
          height: 8,
          borderRadius: "999px",
          background: STATE_COLORS[status.state] ?? "var(--color-fg-subtle)",
          flexShrink: 0,
        }}
      />
      <span className="font-mono" style={{ fontSize: "0.78rem", color: "var(--color-fg)" }}>
        {status.state}
      </span>
      <span style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
        {new Date(status.created_at).toLocaleString()}
        {status.creator ? ` · ${status.creator.login}` : ""}
        {status.description ? ` · ${status.description}` : ""}
        {status.environment_url ? ` · ${status.environment_url}` : ""}
      </span>
    </li>
  );
}

// ─── Environments + protection rules (read-only) ─────────────────────────

function EnvironmentsTab({ owner, repo }: { owner: string; repo: string }) {
  const envsQ = useQuery({
    queryKey: ["environments-detail", owner, repo],
    queryFn: () => fetchEnvironmentsDetail(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (envsQ.isLoading) return <Spinner label="loading environments" />;
  if (envsQ.isError)
    return <InlineError title="Failed to load environments" detail={String(envsQ.error)} />;

  const envs = envsQ.data ?? [];
  if (envs.length === 0) return <Blankslate title="No environments" />;

  return (
    <Box>
      {envs.map((env, i) => (
        <EnvironmentRow
          key={env.id}
          owner={owner}
          repo={repo}
          env={env}
          last={i === envs.length - 1}
        />
      ))}
    </Box>
  );
}

function EnvironmentRow({
  owner,
  repo,
  env,
  last,
}: {
  owner: string;
  repo: string;
  env: GithubEnvironmentDetail;
  last: boolean;
}) {
  const [open, setOpen] = useState(false);
  const policy = env.deployment_branch_policy;
  const policySummary =
    policy === null
      ? "all branches may deploy"
      : policy.protected_branches
        ? "protected branches only"
        : "custom branch policies";

  return (
    <div style={{ borderBottom: last ? "none" : "1px solid var(--color-border)" }}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 text-left"
        style={{ padding: "0.7rem 1rem", background: "transparent", border: "none" }}
      >
        {open ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
        <div className="min-w-0 flex-1">
          <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
            {env.name}
          </div>
          <div style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
            {env.protection_rules.length} protection rule
            {env.protection_rules.length === 1 ? "" : "s"} · {policySummary}
          </div>
        </div>
      </button>
      {open && <EnvironmentDetail owner={owner} repo={repo} env={env} />}
    </div>
  );
}

function EnvironmentDetail({
  owner,
  repo,
  env,
}: {
  owner: string;
  repo: string;
  env: GithubEnvironmentDetail;
}) {
  const branchPoliciesQ = useQuery({
    queryKey: ["env-branch-policies", owner, repo, env.name],
    queryFn: () => fetchEnvBranchPolicies(owner, repo, env.name),
  });
  const customRulesQ = useQuery({
    queryKey: ["env-protection-rules", owner, repo, env.name],
    queryFn: () => fetchEnvProtectionRules(owner, repo, env.name),
  });

  return (
    <div
      style={{
        padding: "0 1rem 0.9rem 2.4rem",
        fontSize: "0.8rem",
        color: "var(--color-fg)",
        display: "flex",
        flexDirection: "column",
        gap: "0.5rem",
      }}
    >
      <div>
        <div style={{ fontWeight: 600, marginBottom: "0.2rem" }}>Protection rules</div>
        {env.protection_rules.length === 0 ? (
          <div style={{ color: "var(--color-fg-muted)" }}>None configured.</div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {env.protection_rules.map((rule) => (
              <li key={rule.id} style={{ padding: "0.15rem 0" }}>
                {rule.type === "wait_timer" && <>Wait timer: {rule.wait_timer} minutes</>}
                {rule.type === "required_reviewers" && (
                  <>
                    Required reviewers:{" "}
                    {(rule.reviewers ?? [])
                      .map((r) => r.reviewer?.login ?? r.type)
                      .join(", ") || "none resolved"}
                  </>
                )}
                {rule.type !== "wait_timer" && rule.type !== "required_reviewers" && rule.type}
              </li>
            ))}
          </ul>
        )}
      </div>
      <div>
        <div style={{ fontWeight: 600, marginBottom: "0.2rem" }}>Deployment branch policies</div>
        {branchPoliciesQ.isLoading ? (
          <Spinner label="loading branch policies" />
        ) : branchPoliciesQ.isError ? (
          <InlineError
            title="Failed to load branch policies"
            detail={String(branchPoliciesQ.error)}
          />
        ) : (branchPoliciesQ.data ?? []).length === 0 ? (
          <div style={{ color: "var(--color-fg-muted)" }}>No branch/tag patterns.</div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {(branchPoliciesQ.data ?? []).map((p) => (
              <li key={p.id} className="font-mono" style={{ padding: "0.15rem 0" }}>
                {p.name} <span style={{ color: "var(--color-fg-muted)" }}>({p.type})</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div>
        <div style={{ fontWeight: 600, marginBottom: "0.2rem" }}>
          Custom deployment protection rules
        </div>
        {customRulesQ.isLoading ? (
          <Spinner label="loading custom rules" />
        ) : customRulesQ.isError ? (
          <InlineError title="Failed to load custom rules" detail={String(customRulesQ.error)} />
        ) : (customRulesQ.data ?? []).length === 0 ? (
          <div style={{ color: "var(--color-fg-muted)" }}>No app-backed rules.</div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {(customRulesQ.data ?? []).map((r) => (
              <li key={r.id} style={{ padding: "0.15rem 0" }}>
                {r.app ? r.app.slug : `rule #${r.id}`} ·{" "}
                {r.enabled ? "enabled" : "disabled"}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

// ─── Pending deployment approvals for waiting workflow runs ───────────────

function ApprovalsTab({ owner, repo }: { owner: string; repo: string }) {
  const runsQ = useQuery({
    queryKey: ["waiting-runs", owner, repo],
    queryFn: () => fetchWorkflowRunsPage(owner, repo, { status: "waiting" }),
    enabled: !!owner && !!repo,
    refetchInterval: 5000,
  });

  if (runsQ.isLoading) return <Spinner label="loading waiting runs" />;
  if (runsQ.isError)
    return <InlineError title="Failed to load waiting runs" detail={String(runsQ.error)} />;

  const runs = runsQ.data?.items ?? [];
  if (runs.length === 0)
    return (
      <Blankslate title="No runs waiting for approval">
        Workflow runs targeting reviewer-protected environments appear here.
      </Blankslate>
    );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      {runs.map((run) => (
        <WaitingRunCard key={run.id} owner={owner} repo={repo} run={run} />
      ))}
    </div>
  );
}

function WaitingRunCard({
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
      void qc.invalidateQueries({ queryKey: ["waiting-runs", owner, repo] });
      void qc.invalidateQueries({ queryKey: ["deployments", owner, repo] });
    },
  });

  return (
    <Box
      header={
        <span style={{ fontWeight: 600, fontSize: "0.88rem" }}>
          {run.name} <span style={{ color: "var(--color-fg-subtle)", fontWeight: 400 }}>#{run.run_number}</span>
          <span className="ml-2 font-mono" style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
            {run.head_branch}
          </span>
        </span>
      }
    >
      <div style={{ padding: "0.75rem 1rem" }}>
        {pendingQ.isLoading ? (
          <Spinner label="loading pending deployments" />
        ) : pendingQ.isError ? (
          <InlineError
            title="Failed to load pending deployments"
            detail={String(pendingQ.error)}
          />
        ) : (pendingQ.data ?? []).length === 0 ? (
          <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
            No pending environment deployments for this run.
          </div>
        ) : (
          <>
            <div style={{ fontSize: "0.85rem" }}>
              Waiting to deploy to{" "}
              <span className="font-mono">
                {(pendingQ.data ?? []).map((p) => p.environment.name).join(", ")}
              </span>
            </div>
            <FormLabel id={`approval-comment-${run.id}`}>Comment (optional)</FormLabel>
            <textarea
              id={`approval-comment-${run.id}`}
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
        )}
      </div>
    </Box>
  );
}
