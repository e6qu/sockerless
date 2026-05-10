import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchStorageAccounts, type StorageAccount } from "../api.js";

const columns: ColumnDef<StorageAccount, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "kind", header: "Kind" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function StorageAccountsPage() {
  return (
    <ResourceListPage<StorageAccount>
      kicker="azure · simulator · storage"
      title={<>Accounts</>}
      countNoun="account"
      columns={columns}
      queryKey={["storage-accounts"]}
      queryFn={fetchStorageAccounts}
      filterPlaceholder="Filter accounts…"
      emptyMessage="No storage accounts tracked."
    />
  );
}
