import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchACRRegistries, type ACRRegistry } from "../api.js";

const columns: ColumnDef<ACRRegistry, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "id", header: "Resource ID" },
];

export function ACRRegistriesPage() {
  return (
    <ResourceListPage<ACRRegistry>
      kicker="azure · simulator · acr"
      title={<>Registries</>}
      countNoun="registry"
      columns={columns}
      queryKey={["acr-registries"]}
      queryFn={fetchACRRegistries}
      filterPlaceholder="Filter registries…"
      emptyMessage="No ACR registries tracked."
    />
  );
}
