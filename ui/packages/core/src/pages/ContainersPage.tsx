import { createColumnHelper } from "@tanstack/react-table";
import { useContainers } from "../hooks/index.js";
import { DataTable } from "../components/DataTable.js";
import { StatusBadge } from "../components/StatusBadge.js";
import { Spinner } from "../components/Spinner.js";
import { RefreshButton } from "../components/RefreshButton.js";
import type { ContainerSummary } from "../api/index.js";

const col = createColumnHelper<ContainerSummary>();

const columns = [
  col.accessor("id", {
    header: "ID",
    cell: (info) => info.getValue().slice(0, 12),
  }),
  col.accessor("name", { header: "Name" }),
  col.accessor("image", { header: "Image" }),
  col.accessor("state", {
    header: "State",
    cell: (info) => <StatusBadge status={info.getValue()} />,
  }),
  col.accessor("created", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
  col.accessor("pod_name", { header: "Pod" }),
];

export function ContainersPage() {
  const { data, isLoading, refetch, isFetching } = useContainers();

  if (isLoading) return <Spinner />;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Containers</h2>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>
      <DataTable data={data ?? []} columns={columns} filterPlaceholder="Filter containers..." />
    </div>
  );
}
