import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { fetchCloudFunctions, type CloudFunction } from "../api.js";

const columns: ColumnDef<CloudFunction, any>[] = [
  { accessorKey: "name", header: "Name" },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ getValue }) => <StatusBadge status={getValue<string>()} />,
  },
  { accessorKey: "environment", header: "Environment" },
  { accessorKey: "createTime", header: "Created" },
];

export function CloudFunctionsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["cloud-functions"], queryFn: fetchCloudFunctions, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Cloud Functions</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
