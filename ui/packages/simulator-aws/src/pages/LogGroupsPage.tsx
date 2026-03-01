import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchCWLogGroups, type CWLogGroup } from "../api.js";

const columns: ColumnDef<CWLogGroup, any>[] = [
  { accessorKey: "name", header: "Name" },
  {
    accessorKey: "creationTime",
    header: "Created",
    cell: ({ getValue }) => new Date(getValue<number>()).toLocaleString(),
    sortDescFirst: true,
  },
  { accessorKey: "retentionInDays", header: "Retention (days)", sortDescFirst: true },
  { accessorKey: "storedBytes", header: "Stored Bytes", sortDescFirst: true },
];

export function LogGroupsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["cw-log-groups"], queryFn: fetchCWLogGroups, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">CloudWatch Log Groups</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
