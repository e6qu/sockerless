import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { useState } from "react";
import {
  dispatchWorkflow,
  fetchWorkflowFiles,
  fetchWorkflows,
} from "../api.js";
import type {
  BleephubWorkflow,
  BleephubWorkflowFile,
} from "../types.js";

type Tab = "workflows" | "runs";

export function WorkflowsPage() {
  const [tab, setTab] = useState<Tab>("workflows");
  return (
    <div>
      <PageHeading
        kicker="actions · workflows"
        title={<>Workflows &amp; runs</>}
        meta={
          tab === "workflows"
            ? "YAML files discovered from git + bleephub-submitted definitions."
            : "Run-level history. Click a row for the per-job timeline."
        }
      />
      <TabRow>
        <TabButton active={tab === "workflows"} onClick={() => setTab("workflows")}>
          Workflows (files)
        </TabButton>
        <TabButton active={tab === "runs"} onClick={() => setTab("runs")}>
          Runs
        </TabButton>
      </TabRow>
      {tab === "workflows" ? <WorkflowsTab /> : <RunsTab />}
    </div>
  );
}

function TabRow({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="mb-5 flex gap-1 -mx-1"
      style={{ borderBottom: "1px solid var(--color-border)" }}
    >
      {children}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="px-4 py-2 font-mono uppercase tracking-[0.12em]"
      style={{
        fontSize: "0.7rem",
        color: active ? "var(--color-fg)" : "var(--color-fg-muted)",
        borderBottom: `2px solid ${active ? "var(--color-accent)" : "transparent"}`,
        marginBottom: "-1px",
        transition: "all 0.12s var(--ease-out-quint)",
      }}
    >
      {children}
    </button>
  );
}

const filesCol = createColumnHelper<BleephubWorkflowFile>();

function WorkflowsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ["workflow_files"],
    queryFn: fetchWorkflowFiles,
    refetchInterval: 5000,
  });
  const [dispatchTarget, setDispatchTarget] = useState<BleephubWorkflowFile | null>(null);

  if (isLoading || !data) return <Spinner label="loading workflows" />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    filesCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
          {info.getValue()}
        </span>
      ),
    }),
    filesCol.accessor("path", {
      header: "Path",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>
      ),
    }),
    filesCol.accessor("repoFullName", { header: "Repo" }),
    filesCol.accessor("state", {
      header: "State",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    filesCol.accessor("source", {
      header: "Source",
      cell: (info) => (
        <span
          className="font-mono uppercase tracking-[0.1em]"
          style={{
            color: "var(--color-fg-subtle)",
            fontSize: "0.65rem",
          }}
        >
          {info.getValue()}
        </span>
      ),
    }),
    filesCol.accessor("updatedAt", {
      header: "Updated",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>
          {new Date(info.getValue()).toLocaleString()}
        </span>
      ),
    }),
    filesCol.display({
      id: "actions",
      header: "",
      cell: (info) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation();
            setDispatchTarget(info.row.original);
          }}
        >
          Run →
        </Button>
      ),
    }),
  ];

  return (
    <>
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter workflow files…"
        emptyMessage="No workflow files yet. Push a .github/workflows/*.yml or POST /api/v3/bleephub/workflow."
      />
      {dispatchTarget && (
        <DispatchDialog
          target={dispatchTarget}
          onClose={() => setDispatchTarget(null)}
        />
      )}
    </>
  );
}

const runsCol = createColumnHelper<BleephubWorkflow>();

function RunsTab() {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });

  if (isLoading || !data) return <Spinner label="loading runs" />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    runsCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
          {info.getValue()}
        </span>
      ),
    }),
    runsCol.accessor("runId", {
      header: "Run #",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>#{info.getValue()}</span>
      ),
    }),
    runsCol.accessor("status", {
      header: "Status",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    runsCol.accessor("result", {
      header: "Result",
      cell: (info) => {
        const v = info.getValue();
        return v ? <StatusBadge status={v} /> : null;
      },
    }),
    runsCol.accessor("eventName", {
      header: "Event",
      cell: (info) => (
        <span
          className="font-mono uppercase tracking-[0.1em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
        >
          {info.getValue()}
        </span>
      ),
    }),
    runsCol.accessor("repoFullName", { header: "Repo" }),
    runsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    runsCol.display({
      id: "jobCount",
      header: "Jobs",
      cell: (info) => (
        <span
          className="tabular-nums"
          style={{ color: "var(--color-fg-muted)" }}
        >
          {Object.keys(info.row.original.jobs).length}
        </span>
      ),
    }),
  ];

  return (
    <DataTable
      data={data}
      columns={columns}
      filterPlaceholder="Filter runs…"
      emptyMessage="No runs yet. Submit a workflow via /api/v3/bleephub/workflow or dispatch one from the Workflows tab."
      onRowClick={(row) => navigate(`/ui/workflows/${row.id}`)}
    />
  );
}

function DispatchDialog({
  target,
  onClose,
}: {
  target: BleephubWorkflowFile;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [ref, setRef] = useState("refs/heads/main");
  const [inputsJSON, setInputsJSON] = useState("{}");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      let inputs: Record<string, string> = {};
      try {
        inputs = JSON.parse(inputsJSON || "{}");
      } catch {
        throw new Error("inputs must be valid JSON");
      }
      await dispatchWorkflow(target.repoFullName, target.id, { ref, inputs });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workflows"] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "color-mix(in oklch, var(--color-fg) 60%, transparent)" }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-md p-6"
        style={{
          background: "var(--color-surface-raised)",
          border: "1px solid var(--color-border-strong)",
          borderRadius: "var(--radius-sm)",
          boxShadow: "8px 8px 0 var(--color-accent)",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="mb-1 text-[10px] uppercase tracking-[0.22em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          dispatch workflow
        </div>
        <h3
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "1.6rem",
            letterSpacing: "-0.025em",
            lineHeight: 1.05,
          }}
        >
          {target.name}
        </h3>
        <div
          className="mt-1 mb-5 font-mono text-xs"
          style={{ color: "var(--color-fg-muted)" }}
        >
          {target.path} · {target.repoFullName}
        </div>

        <label
          htmlFor="dispatch-ref"
          className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          Ref
        </label>
        <input
          id="dispatch-ref"
          type="text"
          value={ref}
          onChange={(e) => setRef(e.target.value)}
          className="mb-4 w-full"
        />

        <label
          htmlFor="dispatch-inputs"
          className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          Inputs (JSON)
        </label>
        <textarea
          id="dispatch-inputs"
          value={inputsJSON}
          onChange={(e) => setInputsJSON(e.target.value)}
          rows={5}
          className="mb-4 w-full"
          style={{ resize: "vertical" }}
        />

        {error && (
          <div
            className="mb-4 px-3 py-2 font-mono text-xs"
            style={{
              background: "var(--color-status-error-soft)",
              color: "var(--color-status-error)",
              border: "1px solid var(--color-status-error)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2">
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
            {mutation.isPending ? "Dispatching…" : "Dispatch ⚡"}
          </Button>
        </div>
      </div>
    </div>
  );
}
