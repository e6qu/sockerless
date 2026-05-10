import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchFunctionSites, type FunctionSite } from "../api.js";

const columns: ColumnDef<FunctionSite, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "kind", header: "Kind" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function AzureFunctionsPage() {
  return (
    <ResourceListPage<FunctionSite>
      kicker="azure · simulator · functions"
      title={<>Sites</>}
      countNoun="site"
      columns={columns}
      queryKey={["azf-sites"]}
      queryFn={fetchFunctionSites}
      filterPlaceholder="Filter sites…"
      emptyMessage="No function sites tracked."
    />
  );
}
