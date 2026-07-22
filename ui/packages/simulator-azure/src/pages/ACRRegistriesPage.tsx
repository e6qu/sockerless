import { AzureResourceTable, type AzureColumn } from "../portal/index.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchACRRegistries, type ACRRegistry } from "../api.js";

const columns: AzureColumn<ACRRegistry>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "resourceGroup", header: "Resource group", cell: (row) => resourceGroupOf(row.id), value: (row) => resourceGroupOf(row.id) },
  { id: "location", header: "Location", cell: (row) => locationLabel(row.location), value: (row) => row.location },
  { id: "loginServer", header: "Login server", cell: (row) => `${row.name}.azurecr.io`, value: (row) => `${row.name}.azurecr.io` },
];

export function ACRRegistriesPage() {
  return (
    <AzureResourceTable<ACRRegistry>
      columns={columns}
      queryKey={["acr-registries"]}
      queryFn={fetchACRRegistries}
      filterPlaceholder="Filter by name"
      resourceNoun="container registries"
      emptyTitle="No container registries to display"
      emptyDescription="Registries created in this subscription appear here."
      rowKey={(row) => row.id}
      essentials={(rows) => [
        { label: "Subscription", value: "Simulator" },
        { label: "Registries", value: String(rows.length) },
        { label: "Locations", value: new Set(rows.map((row) => row.location)).size || "—" },
      ]}
    />
  );
}
