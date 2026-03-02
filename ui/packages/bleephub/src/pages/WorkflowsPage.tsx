import { useQuery } from "@tanstack/react-query";
import { DataTable, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { fetchWorkflows } from "../api.js";
import type { BleephubWorkflow } from "../types.js";

const col = createColumnHelper<BleephubWorkflow>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.accessor("name", { header: "Name" }),
  col.accessor("runId", { header: "Run ID" }),
  col.accessor("status", {
    header: "Status",
    cell: (info) => <StatusBadge status={info.getValue()} />,
  }),
  col.accessor("result", {
    header: "Result",
    cell: (info) => {
      const val = info.getValue();
      return val ? <StatusBadge status={val} /> : null;
    },
  }),
  col.accessor("eventName", { header: "Event" }),
  col.accessor("repoFullName", { header: "Repo" }),
  col.accessor("createdAt", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
  col.display({
    id: "jobCount",
    header: "Jobs",
    cell: (info) => Object.keys(info.row.original.jobs).length,
  }),
];

export function WorkflowsPage() {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Workflows ({data.length})</h2>
      <div onClick={(e) => {
        const row = (e.target as HTMLElement).closest("tr");
        if (!row) return;
        const idx = row.dataset.rowIndex ?? row.rowIndex - 1;
        const wf = data[Number(idx)];
        if (wf) navigate(`/ui/workflows/${wf.id}`);
      }}>
        <DataTable data={data} columns={columns} filterPlaceholder="Filter workflows..." />
      </div>
    </div>
  );
}
