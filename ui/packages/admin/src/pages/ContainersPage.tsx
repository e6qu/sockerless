import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { type ColumnDef } from "@tanstack/react-table";
import { AdminApiClient, type AdminContainer } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

const columns: ColumnDef<AdminContainer>[] = [
  {
    accessorKey: "backend",
    header: "Backend",
    cell: ({ getValue }) => (
      <span style={{ color: "var(--color-accent)" }}>{getValue() as string}</span>
    ),
  },
  {
    accessorKey: "id",
    header: "ID",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {(getValue() as string).substring(0, 12)}
      </span>
    ),
  },
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
    accessorKey: "image",
    header: "Image",
    cell: ({ getValue }) => (
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {getValue() as string}
      </span>
    ),
  },
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
    refetchInterval: 5000,
  });

  if (isLoading) return <Spinner label="loading containers" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!data) return <Spinner label="loading containers" />;

  return (
    <div>
      <PageHeading
        kicker="admin · containers"
        title={<>Containers</>}
        meta={`${data.length} container${data.length === 1 ? "" : "s"} across all backends`}
      />
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter containers…"
        emptyMessage="No containers found across any backend."
      />
    </div>
  );
}
