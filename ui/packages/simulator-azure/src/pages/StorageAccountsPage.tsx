import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchStorageAccounts, type StorageAccount } from "../api.js";

const columns: ColumnDef<StorageAccount, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "kind", header: "Kind" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function StorageAccountsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["storage-accounts"], queryFn: fetchStorageAccounts, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Storage Accounts</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
