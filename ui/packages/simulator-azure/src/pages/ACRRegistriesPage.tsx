import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchACRRegistries, type ACRRegistry } from "../api.js";

const columns: ColumnDef<ACRRegistry, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function ACRRegistriesPage() {
  const { data, isLoading } = useQuery({ queryKey: ["acr-registries"], queryFn: fetchACRRegistries, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">ACR Registries</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
