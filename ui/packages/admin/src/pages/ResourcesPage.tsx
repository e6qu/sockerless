import { useQuery } from "@tanstack/react-query";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminResource } from "../api.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminResource, any>[] = [
  { accessorKey: "backend", header: "Backend" },
  { accessorKey: "resourceType", header: "Type" },
  { accessorKey: "resourceId", header: "Resource ID" },
  { accessorKey: "containerId", header: "Container" },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => {
      const s = getValue() as string | undefined;
      return s ? <StatusBadge status={s} /> : "-";
    },
  },
  { accessorKey: "createdAt", header: "Created" },
];

export function ResourcesPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["resources"],
    queryFn: () => api.resources(),
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Cloud Resources ({data.length})</h2>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter resources..." />
    </div>
  );
}
