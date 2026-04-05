import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  StatusBadge,
  Spinner,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminContainer } from "../api.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminContainer>[] = [
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
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["containers"],
    queryFn: () => api.containers(),
  });

  if (isLoading) return <Spinner />;
  if (isError)
    return (
      <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
        Error: {error?.message ?? "Failed to load"}
      </div>
    );
  if (!data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Containers ({data.length})</h2>
      {data.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          No containers found.
        </p>
      ) : (
        <DataTable
          data={data}
          columns={columns}
          filterPlaceholder="Filter containers..."
        />
      )}
    </div>
  );
}
