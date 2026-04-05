import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  StatusBadge,
  Spinner,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type ContextInfo } from "../api.js";

const api = new AdminApiClient();

const columns: ColumnDef<ContextInfo, string>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "backend", header: "Backend" },
  {
    accessorKey: "active",
    header: "Active",
    cell: ({ getValue }) => (getValue() ? <StatusBadge status="ok" /> : "-"),
  },
  {
    accessorKey: "backend_addr",
    header: "Backend Address",
    cell: ({ getValue }) => (getValue() as string) || "-",
  },
  {
    accessorKey: "frontend_addr",
    header: "Frontend Address",
    cell: ({ getValue }) => (getValue() as string) || "-",
  },
];

export function ContextsPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["contexts"],
    queryFn: () => api.contexts(),
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
      <h2 className="text-xl font-semibold">CLI Contexts</h2>
      {data.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          No contexts found.
        </p>
      ) : (
        <DataTable
          data={data}
          columns={columns}
          filterPlaceholder="Filter contexts..."
        />
      )}
    </div>
  );
}
