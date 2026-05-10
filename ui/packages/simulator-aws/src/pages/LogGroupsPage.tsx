import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchCWLogGroups, type CWLogGroup } from "../api.js";

const columns: ColumnDef<CWLogGroup, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  {
    accessorKey: "creationTime",
    header: "Created",
    cell: ({ getValue }) => new Date(getValue<number>()).toLocaleString(),
    sortDescFirst: true,
  },
  {
    accessorKey: "retentionInDays",
    header: "Retention (days)",
    sortDescFirst: true,
  },
  { accessorKey: "storedBytes", header: "Stored Bytes", sortDescFirst: true },
];

export function LogGroupsPage() {
  return (
    <ResourceListPage<CWLogGroup>
      kicker="aws · simulator · cloudwatch"
      title={<>Log groups</>}
      countNoun="log group"
      columns={columns}
      queryKey={["cw-log-groups"]}
      queryFn={fetchCWLogGroups}
      filterPlaceholder="Filter log groups…"
      emptyMessage="No log groups tracked."
    />
  );
}
