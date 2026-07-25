import { Link } from "react-router";
import { AzureResourceTable, type AzureColumn } from "../portal/index.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchStorageAccounts, type StorageAccount } from "../api.js";

const columns: AzureColumn<StorageAccount>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link to={`/ui/storage/${encodeURIComponent(row.name)}`}>{row.name}</Link>,
    value: (row) => row.name,
  },
  { id: "resourceGroup", header: "Resource group", cell: (row) => resourceGroupOf(row.id), value: (row) => resourceGroupOf(row.id) },
  { id: "location", header: "Location", cell: (row) => locationLabel(row.location), value: (row) => row.location },
  { id: "kind", header: "Kind", cell: (row) => row.kind, value: (row) => row.kind },
];

export function StorageAccountsPage() {
  return (
    <AzureResourceTable<StorageAccount>
      columns={columns}
      queryKey={["storage-accounts"]}
      queryFn={fetchStorageAccounts}
      filterPlaceholder="Filter by name"
      resourceNoun="storage accounts"
      emptyTitle="No storage accounts to display"
      emptyDescription="Storage accounts created in this subscription appear here."
      rowKey={(row) => row.id}
      essentials={(rows) => [
        { label: "Subscription", value: "Simulator" },
        { label: "Storage accounts", value: String(rows.length) },
        { label: "Locations", value: new Set(rows.map((row) => row.location)).size || "—" },
      ]}
    />
  );
}
