import { createColumnHelper } from "@tanstack/react-table";
import { useResources } from "../hooks/index.js";
import { DataTable } from "../components/DataTable.js";
import { StatusBadge } from "../components/StatusBadge.js";
import { Spinner } from "../components/Spinner.js";
import { RefreshButton } from "../components/RefreshButton.js";
import type { ResourceEntry } from "../api/index.js";

const col = createColumnHelper<ResourceEntry>();

const columns = [
  col.accessor("containerId", {
    header: "Container",
    cell: (info) => info.getValue().slice(0, 12),
  }),
  col.accessor("backend", { header: "Backend" }),
  col.accessor("resourceType", { header: "Type" }),
  col.accessor("resourceId", { header: "Resource ID" }),
  col.accessor("status", {
    header: "Status",
    cell: (info) => {
      const v = info.getValue();
      return v ? <StatusBadge status={v} /> : "—";
    },
  }),
  col.accessor("createdAt", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
];

export function ResourcesPage() {
  const { data, isLoading, refetch, isFetching } = useResources();

  if (isLoading) return <Spinner />;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Resources</h2>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>
      <DataTable data={data ?? []} columns={columns} filterPlaceholder="Filter resources..." />
    </div>
  );
}
