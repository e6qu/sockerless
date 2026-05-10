import { type ColumnDef } from "@tanstack/react-table";
import {
  ResourceListPage,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { fetchLambdaFunctions, type LambdaFunction } from "../api.js";

const columns: ColumnDef<LambdaFunction, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "runtime", header: "Runtime" },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ getValue }) => <StatusBadge status={getValue<string>()} />,
  },
  { accessorKey: "memorySize", header: "Memory (MB)", sortDescFirst: true },
  { accessorKey: "timeout", header: "Timeout (s)", sortDescFirst: true },
  { accessorKey: "lastModified", header: "Last Modified" },
];

export function LambdaFunctionsPage() {
  return (
    <ResourceListPage<LambdaFunction>
      kicker="aws · simulator · lambda"
      title={<>Functions</>}
      countNoun="function"
      columns={columns}
      queryKey={["lambda-functions"]}
      queryFn={fetchLambdaFunctions}
      filterPlaceholder="Filter functions…"
      emptyMessage="No Lambda functions tracked."
    />
  );
}
