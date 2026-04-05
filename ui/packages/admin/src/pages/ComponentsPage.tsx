import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import {
  DataTable,
  StatusBadge,
  Spinner,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminComponent } from "../api.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminComponent>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "type", header: "Type" },
  { accessorKey: "addr", header: "Address" },
  {
    accessorKey: "health",
    header: "Health",
    cell: ({ getValue }) => (
      <StatusBadge
        status={
          getValue() === "up"
            ? "ok"
            : getValue() === "unknown"
              ? "warning"
              : "error"
        }
      />
    ),
  },
  {
    accessorKey: "uptime",
    header: "Uptime",
    cell: ({ getValue }) => {
      const secs = getValue() as number;
      if (secs === 0) return "-";
      const h = Math.floor(secs / 3600);
      const m = Math.floor((secs % 3600) / 60);
      return h > 0 ? `${h}h ${m}m` : `${m}m`;
    },
    sortDescFirst: true,
  },
];

export function ComponentsPage() {
  const navigate = useNavigate();
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
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
      <h2 className="text-xl font-semibold">Components</h2>
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter components..."
        onRowClick={(comp) =>
          navigate(`/ui/components/${encodeURIComponent(comp.name)}`)
        }
      />
    </div>
  );
}
