import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DataTable, Spinner, StatusBadge } from "@sockerless/ui-core/components";
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
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Workflows</h2>
      <div className="flex gap-2 border-b border-gray-200">
        <TabButton active={tab === "workflows"} onClick={() => setTab("workflows")}>
          Workflows (files)
        </TabButton>
        <TabButton active={tab === "runs"} onClick={() => setTab("runs")}>
          Runs
        </TabButton>
      </div>
      {tab === "workflows" ? <WorkflowsTab /> : <RunsTab />}
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
  const cls = active
    ? "px-4 py-2 border-b-2 border-blue-600 text-blue-600 font-medium"
    : "px-4 py-2 border-b-2 border-transparent text-gray-600 hover:text-gray-900";
  return (
    <button type="button" onClick={onClick} className={cls}>
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

  if (isLoading || !data) return <Spinner />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    filesCol.accessor("name", { header: "Name" }),
    filesCol.accessor("path", { header: "Path" }),
    filesCol.accessor("repoFullName", { header: "Repo" }),
    filesCol.accessor("state", {
      header: "State",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    filesCol.accessor("source", {
      header: "Source",
      cell: (info) => (
        <span className="text-xs text-gray-500">{info.getValue()}</span>
      ),
    }),
    filesCol.accessor("updatedAt", {
      header: "Updated",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    filesCol.display({
      id: "actions",
      header: "Actions",
      cell: (info) => (
        <button
          type="button"
          className="text-blue-600 hover:text-blue-800 text-sm font-medium"
          onClick={() => setDispatchTarget(info.row.original)}
        >
          Run workflow
        </button>
      ),
    }),
  ];

  return (
    <>
      <div className="text-sm text-gray-600">
        {data.length} workflow file{data.length === 1 ? "" : "s"}. Discovered from
        repo-on-disk and from <code>POST /api/v3/bleephub/workflow</code> submissions.
      </div>
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter workflow files..."
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

  if (isLoading || !data) return <Spinner />;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    runsCol.accessor("name", { header: "Name" }),
    runsCol.accessor("runId", { header: "Run ID" }),
    runsCol.accessor("status", {
      header: "Status",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    runsCol.accessor("result", {
      header: "Result",
      cell: (info) => {
        const val = info.getValue();
        return val ? <StatusBadge status={val} /> : null;
      },
    }),
    runsCol.accessor("eventName", { header: "Event" }),
    runsCol.accessor("repoFullName", { header: "Repo" }),
    runsCol.accessor("createdAt", {
      header: "Created",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    runsCol.display({
      id: "jobCount",
      header: "Jobs",
      cell: (info) => Object.keys(info.row.original.jobs).length,
    }),
  ];

  return (
    <div
      onClick={(e) => {
        const row = (e.target as HTMLElement).closest("tr");
        if (!row) return;
        const idx = row.dataset.rowIndex ?? row.rowIndex - 1;
        const wf = data[Number(idx)];
        if (wf) navigate(`/ui/workflows/${wf.id}`);
      }}
    >
      <div className="text-sm text-gray-600 mb-2">{data.length} run{data.length === 1 ? "" : "s"}</div>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter runs..." />
    </div>
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
      } catch (e) {
        throw new Error("inputs must be valid JSON");
      }
      await dispatchWorkflow(target.repoFullName, target.id, { ref, inputs });
    },
    onSuccess: () => {
      // Refresh the runs list so the new run appears.
      queryClient.invalidateQueries({ queryKey: ["workflows"] });
      onClose();
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl p-6 max-w-md w-full space-y-4">
        <div>
          <h3 className="text-lg font-semibold">Run workflow</h3>
          <div className="text-sm text-gray-600">
            <code>{target.path}</code> · {target.repoFullName}
          </div>
        </div>
        <div>
          <label htmlFor="dispatch-ref" className="block text-sm font-medium text-gray-700">
            Ref
          </label>
          <input
            id="dispatch-ref"
            type="text"
            value={ref}
            onChange={(e) => setRef(e.target.value)}
            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm font-mono text-sm"
            placeholder="refs/heads/main"
          />
        </div>
        <div>
          <label htmlFor="dispatch-inputs" className="block text-sm font-medium text-gray-700">
            Inputs (JSON)
          </label>
          <textarea
            id="dispatch-inputs"
            value={inputsJSON}
            onChange={(e) => setInputsJSON(e.target.value)}
            rows={4}
            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm font-mono text-sm"
            placeholder='{"key":"value"}'
          />
        </div>
        {error && (
          <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 hover:bg-gray-50"
            disabled={mutation.isPending}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              setError(null);
              mutation.mutate();
            }}
            className="px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50"
            disabled={mutation.isPending}
          >
            {mutation.isPending ? "Dispatching…" : "Dispatch"}
          </button>
        </div>
      </div>
    </div>
  );
}
