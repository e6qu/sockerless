import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type ContextInfo } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const columns: ColumnDef<ContextInfo, string>[] = [
  {
    accessorKey: "name",
    header: "Name",
    cell: ({ getValue }) => (
      <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
        {getValue() as string}
      </span>
    ),
  },
  {
    accessorKey: "backend",
    header: "Backend",
    cell: ({ getValue }) => (
      <span style={{ color: "var(--color-accent)" }}>{getValue() as string}</span>
    ),
  },
  {
    accessorKey: "active",
    header: "Active",
    cell: ({ getValue }) =>
      getValue() ? (
        <StatusBadge status="active" />
      ) : (
        <span style={{ color: "var(--color-fg-subtle)" }}>—</span>
      ),
  },
  {
    accessorKey: "backend_addr",
    header: "Backend address",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {(getValue() as string) || "—"}
      </span>
    ),
  },
  {
    accessorKey: "frontend_addr",
    header: "Frontend address",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {(getValue() as string) || "—"}
      </span>
    ),
  },
];

export function ContextsPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["contexts"],
    queryFn: () => api.contexts(),
  });

  if (isLoading) return <Spinner label="loading contexts" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!data) return <Spinner label="loading contexts" />;

  return (
    <div>
      <PageHeading
        kicker="admin · cli contexts"
        title={<>CLI contexts</>}
        meta={`${data.length} context${data.length === 1 ? "" : "s"} configured`}
      />
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter contexts…"
        emptyMessage="No contexts configured. Run `sockerless context create <name>`."
      />
    </div>
  );
}
