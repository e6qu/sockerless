import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { useState } from "react";
import { dispatchWorkflow, fetchWorkflowFiles, fetchWorkflows } from "../api.js";
import type { BleephubWorkflow, BleephubWorkflowFile } from "../types.js";
import {
  PageTitle,
  Tabs,
  Button,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
} from "../components/ui.js";

type Tab = "workflows" | "runs";

export function WorkflowsPage() {
  const [tab, setTab] = useState<Tab>("workflows");
  return (
    <div>
      <PageTitle
        title="Workflows & runs"
        meta={
          tab === "workflows"
            ? "YAML files discovered from git + bleephub-submitted definitions."
            : "Run-level history. Click a row for the per-job timeline."
        }
      />
      <Tabs
        items={[
          { key: "workflows", label: "Workflows" },
          { key: "runs", label: "Runs" },
        ]}
        active={tab}
        onChange={setTab}
      />
      {tab === "workflows" ? <WorkflowsTab /> : <RunsTab />}
    </div>
  );
}

const filesCol = createColumnHelper<BleephubWorkflowFile>();

function WorkflowsTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["workflow_files"],
    queryFn: fetchWorkflowFiles,
    refetchInterval: 5000,
  });
  const [dispatchTarget, setDispatchTarget] = useState<BleephubWorkflowFile | null>(null);

  if (isError) return <InlineError title="Failed to load workflows" />;
  if (isLoading || !data) return <Spinner label="loading workflows" />;

  const columns = [
    filesCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>
      ),
    }),
    filesCol.accessor("path", {
      header: "Path",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    filesCol.accessor("repoFullName", { header: "Repo" }),
    filesCol.accessor("state", {
      header: "State",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    filesCol.accessor("source", {
      header: "Source",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    filesCol.accessor("updatedAt", {
      header: "Updated",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>{new Date(info.getValue()).toLocaleString()}</span>
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
          Run
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
        emptyMessage="No workflow files yet. Push a .github/workflows/*.yml or POST /internal/exec/workflow."
      />
      {dispatchTarget && (
        <DispatchDialog target={dispatchTarget} onClose={() => setDispatchTarget(null)} />
      )}
    </>
  );
}

const runsCol = createColumnHelper<BleephubWorkflow>();

function RunsTab() {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });

  if (isError) return <InlineError title="Failed to load runs" />;
  if (isLoading || !data) return <Spinner label="loading runs" />;

  const columns = [
    runsCol.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>
      ),
    }),
    runsCol.accessor("runId", {
      header: "Run #",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>#{info.getValue()}</span>,
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
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
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
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
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
      emptyMessage="No runs yet. Submit a workflow via /internal/exec/workflow or dispatch one from the Workflows tab."
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
    <Modal title={`Run ${target.name}`} onClose={onClose}>
      <div className="mb-4" style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
        {target.path} · {target.repoFullName}
      </div>

      <FormLabel id="dispatch-ref">Ref</FormLabel>
      <input
        id="dispatch-ref"
        type="text"
        value={ref}
        onChange={(e) => setRef(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="dispatch-inputs">Inputs (JSON)</FormLabel>
      <textarea
        id="dispatch-inputs"
        value={inputsJSON}
        onChange={(e) => setInputsJSON(e.target.value)}
        rows={5}
        className="mb-4 w-full"
        style={{ resize: "vertical", fontFamily: "var(--font-mono)" }}
      />

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
