import { useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchActionsWorkflows,
  fetchWorkflowRunsPage,
  fetchFileContent,
  fetchEnvironments,
  dispatchWorkflow,
  enableWorkflow,
  disableWorkflow,
  isNotFound,
  type RunFilters,
} from "../api.js";
import type { GithubWorkflow, GithubWorkflowRun, WorkflowDispatchInput } from "../types.js";
import { decodeContentsBase64, parseWorkflowDispatch } from "../utils/workflowDispatch.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { RunStatusIcon } from "../components/RunStatusIcon.js";
import {
  Box,
  Blankslate,
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
} from "../components/ui.js";
import { BranchIcon, KebabIcon, PlayIcon } from "../components/octicons.js";

const RUN_STATUSES = ["queued", "in_progress", "completed", "waiting"] as const;
const RUN_EVENTS = [
  "push",
  "pull_request",
  "workflow_dispatch",
  "repository_dispatch",
  "schedule",
] as const;

export function ActionsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const counts = useOpenCounts(owner, repo);
  const [searchParams, setSearchParams] = useSearchParams();

  const workflowsQ = useQuery({
    queryKey: ["actions-workflows", owner, repo],
    queryFn: () => fetchActionsWorkflows(owner, repo),
    enabled: !!owner && !!repo,
  });

  const selectedId = searchParams.get("workflow");
  const workflows = workflowsQ.data?.items ?? [];
  const selected = selectedId
    ? workflows.find((w) => String(w.id) === selectedId) ?? null
    : null;

  const selectWorkflow = (id: number | null) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (id === null) next.delete("workflow");
      else next.set("workflow", String(id));
      return next;
    });
  };

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="actions" {...counts} />
      <div className="flex flex-wrap items-start gap-6 md:flex-nowrap">
        <aside className="w-full shrink-0 md:w-60">
          <div
            style={{
              fontSize: "0.78rem",
              fontWeight: 600,
              color: "var(--color-fg-muted)",
              padding: "0.3rem 0.5rem",
            }}
          >
            Workflows
          </div>
          {workflowsQ.isLoading && <Spinner label="loading workflows" />}
          {workflowsQ.isError && (
            <InlineError inline title="Failed to load workflows" detail={String(workflowsQ.error)} />
          )}
          {workflowsQ.data && (
            <nav aria-label="Workflows" className="flex flex-col">
              <SidebarItem
                label="All workflows"
                active={selected === null}
                dimmed={false}
                onClick={() => selectWorkflow(null)}
              />
              {workflows.map((w) => (
                <SidebarItem
                  key={w.id}
                  label={w.name}
                  active={selected?.id === w.id}
                  dimmed={w.state !== "active"}
                  onClick={() => selectWorkflow(w.id)}
                />
              ))}
            </nav>
          )}
        </aside>
        <div className="min-w-0 flex-1">
          <RunsPane owner={owner} repo={repo} workflow={selected} />
        </div>
      </div>
    </div>
  );
}

function SidebarItem({
  label,
  active,
  dimmed,
  onClick,
}: {
  label: string;
  active: boolean;
  dimmed: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="text-left"
      style={{
        padding: "0.4rem 0.5rem",
        borderRadius: "var(--radius-md)",
        border: "none",
        fontSize: "0.85rem",
        fontWeight: active ? 600 : 500,
        color: dimmed ? "var(--color-fg-subtle)" : active ? "var(--color-fg)" : "var(--color-fg-muted)",
        background: active ? "color-mix(in srgb, var(--color-fg-muted) 12%, transparent)" : "transparent",
        overflow: "hidden",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
      }}
    >
      {label}
    </button>
  );
}

function RunsPane({
  owner,
  repo,
  workflow,
}: {
  owner: string;
  repo: string;
  workflow: GithubWorkflow | null;
}) {
  const [status, setStatus] = useState("");
  const [event, setEvent] = useState("");
  const [branchInput, setBranchInput] = useState("");
  const [branch, setBranch] = useState("");

  const filters: RunFilters = useMemo(
    () => ({
      workflowId: workflow?.id,
      status: status || undefined,
      branch: branch || undefined,
      event: event || undefined,
    }),
    [workflow?.id, status, branch, event],
  );

  const runsQ = useInfiniteQuery({
    queryKey: ["runs", owner, repo, filters],
    queryFn: ({ pageParam }) => fetchWorkflowRunsPage(owner, repo, filters, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextUrl ?? undefined,
    enabled: !!owner && !!repo,
  });
  const runs = runsQ.data?.pages.flatMap((p) => p.items) ?? [];
  const totalCount = runsQ.data?.pages[0]?.totalCount;

  return (
    <div>
      {workflow && <WorkflowHeader owner={owner} repo={repo} workflow={workflow} />}

      <form
        className="mb-3 flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setBranch(branchInput.trim());
        }}
      >
        <label htmlFor="runs-event-filter" className="sr-only">
          Event
        </label>
        <select
          id="runs-event-filter"
          value={event}
          onChange={(e) => setEvent(e.target.value)}
          style={filterControlStyle}
        >
          <option value="">Event: any</option>
          {RUN_EVENTS.map((ev) => (
            <option key={ev} value={ev}>
              {ev}
            </option>
          ))}
        </select>
        <label htmlFor="runs-status-filter" className="sr-only">
          Status
        </label>
        <select
          id="runs-status-filter"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
          style={filterControlStyle}
        >
          <option value="">Status: any</option>
          {RUN_STATUSES.map((st) => (
            <option key={st} value={st}>
              {st}
            </option>
          ))}
        </select>
        <label htmlFor="runs-branch-filter" className="sr-only">
          Branch
        </label>
        <input
          id="runs-branch-filter"
          type="text"
          placeholder="Filter by branch…"
          value={branchInput}
          onChange={(e) => setBranchInput(e.target.value)}
          onBlur={() => setBranch(branchInput.trim())}
          style={{ ...filterControlStyle, minWidth: "11rem" }}
        />
        {totalCount !== undefined && (
          <span className="ml-auto" style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
            {totalCount} workflow run{totalCount === 1 ? "" : "s"}
          </span>
        )}
      </form>

      {runsQ.isLoading && <Spinner label="loading workflow runs" />}
      {runsQ.isError && (
        <InlineError title="Failed to load workflow runs" detail={String(runsQ.error)} />
      )}
      {runsQ.data &&
        (runs.length === 0 ? (
          <Blankslate icon={<PlayIcon size={26} />} title="There are no workflow runs yet">
            Runs triggered by push, pull request or manual dispatch will show up here.
          </Blankslate>
        ) : (
          <>
            <Box>
              {runs.map((run, i) => (
                <RunRow
                  key={run.id}
                  owner={owner}
                  repo={repo}
                  run={run}
                  last={i === runs.length - 1}
                />
              ))}
            </Box>
            {runsQ.hasNextPage && (
              <div className="mt-3 flex justify-center">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={runsQ.isFetchingNextPage}
                  onClick={() => void runsQ.fetchNextPage()}
                >
                  {runsQ.isFetchingNextPage ? "Loading…" : "Load more"}
                </Button>
              </div>
            )}
          </>
        ))}
    </div>
  );
}

const filterControlStyle: React.CSSProperties = {
  padding: "0.28rem 0.55rem",
  fontSize: "0.8rem",
  background: "var(--color-bg-subtle)",
  color: "var(--color-fg)",
  border: "1px solid var(--color-border)",
  borderRadius: "var(--radius-md)",
};

function RunRow({
  owner,
  repo,
  run,
  last,
}: {
  owner: string;
  repo: string;
  run: GithubWorkflowRun;
  last: boolean;
}) {
  return (
    <Link
      to={`/ui/repos/${owner}/${repo}/actions/runs/${run.id}`}
      className="flex items-start gap-2.5"
      style={{
        padding: "0.7rem 1rem",
        borderBottom: last ? "none" : "1px solid var(--color-border)",
        textDecoration: "none",
      }}
    >
      <span style={{ marginTop: "0.1rem" }}>
        <RunStatusIcon status={run.status} conclusion={run.conclusion} />
      </span>
      <div className="min-w-0 flex-1">
        <div style={{ fontSize: "0.92rem", fontWeight: 600, color: "var(--color-fg)" }}>
          {run.name}
        </div>
        <div
          className="mt-1 flex flex-wrap items-center gap-x-2"
          style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}
        >
          <span>#{run.run_number}</span>
          <span>{run.event}</span>
          {run.actor && <span>by {run.actor.login}</span>}
          <span>{new Date(run.created_at).toLocaleString()}</span>
        </div>
      </div>
      <span
        className="inline-flex shrink-0 items-center gap-1 font-mono"
        style={{
          fontSize: "0.74rem",
          color: "var(--color-accent)",
          background: "var(--color-accent-soft)",
          padding: "0.12rem 0.5rem",
          borderRadius: "2rem",
        }}
      >
        <BranchIcon size={12} /> {run.head_branch}
      </span>
    </Link>
  );
}

// ─── Selected-workflow header: dispatch + enable/disable ────────────────

function WorkflowHeader({
  owner,
  repo,
  workflow,
}: {
  owner: string;
  repo: string;
  workflow: GithubWorkflow;
}) {
  const qc = useQueryClient();
  const [menuOpen, setMenuOpen] = useState(false);
  const [dispatchOpen, setDispatchOpen] = useState(false);
  const disabled = workflow.state !== "active";

  // The dispatch form only exists for files with `on: workflow_dispatch` —
  // read the workflow file through the GitHub Contents application programming
  // interface and parse the trigger section.
  const yamlQ = useQuery({
    queryKey: ["workflow-yaml", owner, repo, workflow.path],
    queryFn: () => fetchFileContent(owner, repo, workflow.path),
    enabled: !!workflow.path,
  });
  const dispatchSpec = useMemo(() => {
    if (!yamlQ.data) return null;
    return parseWorkflowDispatch(decodeContentsBase64(yamlQ.data.content));
  }, [yamlQ.data]);

  const toggleMutation = useMutation({
    mutationFn: () =>
      disabled ? enableWorkflow(owner, repo, workflow.id) : disableWorkflow(owner, repo, workflow.id),
    onSuccess: () => {
      setMenuOpen(false);
      void qc.invalidateQueries({ queryKey: ["actions-workflows", owner, repo] });
    },
  });

  return (
    <div className="mb-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 style={{ fontSize: "1.15rem", fontWeight: 600, color: "var(--color-fg)" }}>
          {workflow.name}{" "}
          <span className="font-mono" style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)", fontWeight: 400 }}>
            {workflow.path}
          </span>
        </h2>
        <div className="relative flex items-center gap-2">
          {dispatchSpec?.hasDispatch && !disabled && (
            <Button variant="primary" size="sm" onClick={() => setDispatchOpen(true)}>
              Run workflow
            </Button>
          )}
          <Button
            variant="secondary"
            size="sm"
            aria-label="Workflow options"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((v) => !v)}
          >
            <KebabIcon size={14} />
          </Button>
          {menuOpen && (
            <div
              role="menu"
              className="absolute right-0 top-full z-10 mt-1"
              style={{
                background: "var(--color-surface-raised)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-md)",
                boxShadow: "0 8px 24px rgba(31,35,40,0.12)",
                minWidth: "13rem",
                padding: "0.25rem",
              }}
            >
              <button
                type="button"
                role="menuitem"
                disabled={toggleMutation.isPending}
                onClick={() => toggleMutation.mutate()}
                className="block w-full text-left"
                style={{
                  padding: "0.4rem 0.6rem",
                  fontSize: "0.82rem",
                  color: "var(--color-fg)",
                  background: "transparent",
                  border: "none",
                  borderRadius: "var(--radius-sm)",
                }}
              >
                {toggleMutation.isPending
                  ? "Saving…"
                  : disabled
                    ? "Enable workflow"
                    : "Disable workflow"}
              </button>
            </div>
          )}
        </div>
      </div>
      {toggleMutation.isError && (
        <div className="mt-2">
          <ErrorBanner>{String(toggleMutation.error)}</ErrorBanner>
        </div>
      )}
      {yamlQ.isError && !isNotFound(yamlQ.error) && (
        <div className="mt-2" style={{ fontSize: "0.78rem", color: "var(--color-status-error)" }}>
          Could not read the workflow file: {String(yamlQ.error)}
        </div>
      )}
      {disabled && (
        <div
          className="mt-2"
          style={{
            padding: "0.5rem 0.75rem",
            background: "var(--color-status-warn-soft)",
            color: "var(--color-status-warn)",
            border: "1px solid color-mix(in srgb, var(--color-status-warn) 40%, transparent)",
            borderRadius: "var(--radius-md)",
            fontSize: "0.82rem",
          }}
        >
          This workflow was disabled{workflow.state === "disabled_manually" ? " manually" : ""}.
          New runs will not be triggered until it is enabled again.
        </div>
      )}
      {dispatchOpen && dispatchSpec && (
        <DispatchFormModal
          owner={owner}
          repo={repo}
          workflow={workflow}
          inputs={dispatchSpec.inputs}
          onClose={() => setDispatchOpen(false)}
        />
      )}
    </div>
  );
}

// ─── Workflow-dispatch form (built from on.workflow_dispatch.inputs) ────

function defaultValueFor(def: WorkflowDispatchInput): string {
  if (def.type === "boolean") {
    return def.default === true || def.default === "true" ? "true" : "false";
  }
  if (typeof def.default === "string") return def.default;
  if (def.type === "choice" && def.options && def.options.length > 0) return def.options[0];
  return "";
}

function DispatchFormModal({
  owner,
  repo,
  workflow,
  inputs,
  onClose,
}: {
  owner: string;
  repo: string;
  workflow: GithubWorkflow;
  inputs: Record<string, WorkflowDispatchInput>;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const inputNames = Object.keys(inputs);
  const [ref, setRef] = useState("main");
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const name of inputNames) init[name] = defaultValueFor(inputs[name]);
    return init;
  });
  const [error, setError] = useState<string | null>(null);

  // `environment`-typed inputs offer the repo's environments as choices.
  const needsEnvs = inputNames.some((n) => inputs[n].type === "environment");
  const envsQ = useQuery({
    queryKey: ["environments", owner, repo],
    queryFn: () => fetchEnvironments(owner, repo),
    enabled: needsEnvs,
  });

  const mutation = useMutation({
    mutationFn: async () => {
      for (const name of inputNames) {
        if (inputs[name].required && values[name] === "") {
          throw new Error(`Input "${name}" is required`);
        }
      }
      await dispatchWorkflow(`${owner}/${repo}`, workflow.id, {
        ref,
        inputs: Object.fromEntries(inputNames.map((n) => [n, values[n]])),
      });
    },
    onSuccess: () => {
      // The new run appears asynchronously — give the server a beat
      // before refreshing the runs list.
      setTimeout(() => {
        void qc.invalidateQueries({ queryKey: ["runs", owner, repo] });
      }, 1000);
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  const setValue = (name: string, v: string) => setValues((prev) => ({ ...prev, [name]: v }));

  return (
    <Modal title={`Run workflow: ${workflow.name}`} onClose={onClose}>
      <FormLabel id="dispatch-ref">Use workflow from branch</FormLabel>
      <input
        id="dispatch-ref"
        type="text"
        value={ref}
        onChange={(e) => setRef(e.target.value)}
        className="mb-4 w-full"
      />

      {inputNames.map((name) => {
        const def = inputs[name];
        const fieldId = `dispatch-input-${name}`;
        const label = (
          <FormLabel id={fieldId}>
            {def.description || name}
            {def.required ? " *" : ""}
          </FormLabel>
        );
        if (def.type === "boolean") {
          return (
            <div key={name} className="mb-4 flex items-center gap-2">
              <input
                id={fieldId}
                type="checkbox"
                checked={values[name] === "true"}
                onChange={(e) => setValue(name, e.target.checked ? "true" : "false")}
              />
              <label htmlFor={fieldId} style={{ fontSize: "0.84rem", color: "var(--color-fg)" }}>
                {def.description || name}
              </label>
            </div>
          );
        }
        if (def.type === "choice") {
          return (
            <div key={name} className="mb-4">
              {label}
              <select
                id={fieldId}
                value={values[name]}
                onChange={(e) => setValue(name, e.target.value)}
                className="w-full"
                style={filterControlStyle}
              >
                {(def.options ?? []).map((opt) => (
                  <option key={opt} value={opt}>
                    {opt}
                  </option>
                ))}
              </select>
            </div>
          );
        }
        if (def.type === "environment") {
          return (
            <div key={name} className="mb-4">
              {label}
              {envsQ.isError ? (
                <ErrorBanner>Failed to load environments: {String(envsQ.error)}</ErrorBanner>
              ) : (
                <select
                  id={fieldId}
                  value={values[name]}
                  onChange={(e) => setValue(name, e.target.value)}
                  className="w-full"
                  style={filterControlStyle}
                >
                  <option value="">{envsQ.isLoading ? "Loading environments…" : "Select environment"}</option>
                  {(envsQ.data ?? []).map((env) => (
                    <option key={env.name} value={env.name}>
                      {env.name}
                    </option>
                  ))}
                </select>
              )}
            </div>
          );
        }
        return (
          <div key={name} className="mb-4">
            {label}
            <input
              id={fieldId}
              type="text"
              value={values[name]}
              onChange={(e) => setValue(name, e.target.value)}
              className="w-full"
            />
          </div>
        );
      })}

      {error && <ErrorBanner>{error}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending}
          variant="primary"
        >
          {mutation.isPending ? "Dispatching…" : "Run workflow"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
