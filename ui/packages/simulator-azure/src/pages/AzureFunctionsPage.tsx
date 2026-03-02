import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchFunctionSites, type FunctionSite } from "../api.js";

const columns: ColumnDef<FunctionSite, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "kind", header: "Kind" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function AzureFunctionsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["azf-sites"], queryFn: fetchFunctionSites, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Azure Functions</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
