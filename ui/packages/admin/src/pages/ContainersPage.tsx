import { useQuery } from "@tanstack/react-query";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminContainer } from "../api.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminContainer, any>[] = [
  { accessorKey: "backend", header: "Backend" },
  {
    accessorKey: "id",
    header: "ID",
    cell: ({ getValue }) => (getValue() as string).substring(0, 12),
  },
  { accessorKey: "name", header: "Name" },
  { accessorKey: "image", header: "Image" },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ getValue }) => <StatusBadge status={getValue() as string} />,
  },
  { accessorKey: "created", header: "Created" },
];

export function ContainersPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["containers"],
    queryFn: () => api.containers(),
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Containers ({data.length})</h2>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter containers..." />
    </div>
  );
}
