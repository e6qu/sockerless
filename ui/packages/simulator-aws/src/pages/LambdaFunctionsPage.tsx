import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { fetchLambdaFunctions, type LambdaFunction } from "../api.js";

const columns: ColumnDef<LambdaFunction, any>[] = [
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
  const { data, isLoading } = useQuery({ queryKey: ["lambda-functions"], queryFn: fetchLambdaFunctions, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Lambda Functions</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
