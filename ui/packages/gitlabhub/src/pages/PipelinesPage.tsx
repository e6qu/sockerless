import { useQuery } from "@tanstack/react-query";
import { DataTable, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { fetchPipelines } from "../api.js";
import type { GitlabhubPipeline } from "../types.js";

const col = createColumnHelper<GitlabhubPipeline>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.accessor("id", {
    header: "ID",
    cell: (info) => `#${info.getValue()}`,
  }),
  col.accessor("project_name", { header: "Project" }),
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
  col.accessor("ref", { header: "Ref" }),
  col.display({
    id: "jobCount",
    header: "Jobs",
    cell: (info) => Object.keys(info.row.original.jobs).length,
  }),
  col.accessor("created_at", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
];

export function PipelinesPage() {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery({
    queryKey: ["pipelines"],
    queryFn: fetchPipelines,
    refetchInterval: 3000,
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Pipelines ({data.length})</h2>
      <div onClick={(e) => {
        const row = (e.target as HTMLElement).closest("tr");
        if (!row) return;
        const idx = row.dataset.rowIndex ?? row.rowIndex - 1;
        const pl = data[Number(idx)];
        if (pl) navigate(`/ui/pipelines/${pl.id}`);
      }}>
        <DataTable data={data} columns={columns} filterPlaceholder="Filter pipelines..." />
      </div>
    </div>
  );
}
