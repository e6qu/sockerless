import { useQuery } from "@tanstack/react-query";
import { DataTable, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchRepos } from "../api.js";
import type { BleephubRepo } from "../types.js";

const col = createColumnHelper<BleephubRepo>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.accessor("full_name", { header: "Full Name" }),
  col.accessor("description", { header: "Description" }),
  col.accessor("default_branch", { header: "Branch" }),
  col.accessor("visibility", {
    header: "Visibility",
    cell: (info) => <StatusBadge status={info.getValue()} />,
  }),
  col.accessor("created_at", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
];

export function ReposPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["repos"],
    queryFn: fetchRepos,
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Repositories ({data.length})</h2>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter repos..." />
    </div>
  );
}
