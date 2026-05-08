import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminResource } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminResource>[] = [
  {
    accessorKey: "backend",
    header: "Backend",
    cell: ({ getValue }) => (
      <span style={{ color: "var(--color-accent)" }}>{getValue() as string}</span>
    ),
  },
  {
    accessorKey: "resourceType",
    header: "Type",
    cell: ({ getValue }) => (
      <span
        className="font-mono uppercase tracking-[0.1em]"
        style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
      >
        {getValue() as string}
      </span>
    ),
  },
  {
    accessorKey: "resourceId",
    header: "Resource ID",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg)" }}>
        {getValue() as string}
      </span>
    ),
  },
  {
    accessorKey: "containerId",
    header: "Container",
    cell: ({ getValue }) => {
      const v = getValue() as string | undefined;
      return (
        <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
          {v ? v.substring(0, 12) : "—"}
        </span>
      );
    },
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => {
      const s = getValue() as string | undefined;
      return s ? <StatusBadge status={s} /> : <span style={{ color: "var(--color-fg-subtle)" }}>—</span>;
    },
  },
  { accessorKey: "createdAt", header: "Created" },
];

export function ResourcesPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["resources"],
    queryFn: () => api.resources(),
    refetchInterval: 5000,
  });

  if (isLoading) return <Spinner label="loading resources" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!data) return <Spinner label="loading resources" />;

  return (
    <div>
      <PageHeading
        kicker="admin · cloud resources"
        title={<>Cloud resources</>}
        meta={`${data.length} entr${data.length === 1 ? "y" : "ies"} tracked across all backends`}
      />
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter resources…"
        emptyMessage="No cloud resources tracked."
      />
    </div>
  );
}
