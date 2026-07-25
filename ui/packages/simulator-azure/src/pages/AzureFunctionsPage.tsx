import { Link } from "react-router";
import { AzureResourceTable, type AzureColumn } from "../portal/index.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchFunctionSites, type FunctionSite } from "../api.js";

const columns: AzureColumn<FunctionSite>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link to={`/ui/functions/${encodeURIComponent(row.name)}`}>{row.name}</Link>,
    value: (row) => row.name,
  },
  { id: "resourceGroup", header: "Resource group", cell: (row) => resourceGroupOf(row.id), value: (row) => resourceGroupOf(row.id) },
  { id: "location", header: "Location", cell: (row) => locationLabel(row.location), value: (row) => row.location },
  { id: "kind", header: "App kind", cell: (row) => row.kind, value: (row) => row.kind },
];

export function AzureFunctionsPage() {
  return (
    <AzureResourceTable<FunctionSite>
      columns={columns}
      queryKey={["fn-sites"]}
      queryFn={fetchFunctionSites}
      filterPlaceholder="Filter by name"
      resourceNoun="Function Apps"
      emptyTitle="No Function Apps to display"
      emptyDescription="Function Apps created in this subscription appear here."
      rowKey={(row) => row.id}
      essentials={(rows) => [
        { label: "Subscription", value: "Simulator" },
        { label: "Function Apps", value: String(rows.length) },
        { label: "Locations", value: new Set(rows.map((row) => row.location)).size || "—" },
      ]}
    />
  );
}
