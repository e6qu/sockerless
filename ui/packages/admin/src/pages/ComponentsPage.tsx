import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import {
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminComponent } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminComponent>[] = [
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
    accessorKey: "type",
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
    accessorKey: "addr",
    header: "Address",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {getValue() as string}
      </span>
    ),
  },
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
      if (!secs || secs === 0) {
        return <span style={{ color: "var(--color-fg-subtle)" }}>—</span>;
      }
      const h = Math.floor(secs / 3600);
      const m = Math.floor((secs % 3600) / 60);
      return (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {h > 0 ? `${h}h ${m}m` : `${m}m`}
        </span>
      );
    },
    sortDescFirst: true,
  },
];

export function ComponentsPage() {
  const navigate = useNavigate();
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
    refetchInterval: 5000,
  });

  if (isLoading) return <Spinner label="loading components" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!data) return <Spinner label="loading components" />;

  return (
    <div>
      <PageHeading
        kicker="admin · components"
        title={<>Components</>}
        meta={`${data.length} component${data.length === 1 ? "" : "s"} registered`}
      />
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter components…"
        emptyMessage="No components registered."
        onRowClick={(comp) =>
          navigate(`/ui/components/${encodeURIComponent(comp.name)}`)
        }
      />
    </div>
  );
}
