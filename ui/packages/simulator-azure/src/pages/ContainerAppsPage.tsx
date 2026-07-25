import { Link } from "react-router";
import { AzureResourceTable, type AzureColumn } from "../portal/index.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchContainerAppJobs, type ContainerAppJob } from "../api.js";

const columns: AzureColumn<ContainerAppJob>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link to={`/ui/container-apps/${encodeURIComponent(row.name)}`}>{row.name}</Link>,
    value: (row) => row.name,
  },
  { id: "resourceGroup", header: "Resource group", cell: (row) => resourceGroupOf(row.id), value: (row) => resourceGroupOf(row.id) },
  { id: "location", header: "Location", cell: (row) => locationLabel(row.location), value: (row) => row.location },
  { id: "type", header: "Type", cell: (row) => row.type, value: (row) => row.type },
];

export function ContainerAppsPage() {
  return (
    <AzureResourceTable<ContainerAppJob>
      columns={columns}
      queryKey={["ca-jobs"]}
      queryFn={fetchContainerAppJobs}
      filterPlaceholder="Filter by name"
      resourceNoun="Container Apps jobs"
      emptyTitle="No Container Apps jobs to display"
      emptyDescription="Jobs created in this subscription appear here."
      rowKey={(row) => row.id}
      essentials={(rows) => [
        { label: "Subscription", value: "Simulator" },
        { label: "Jobs", value: String(rows.length) },
        { label: "Locations", value: new Set(rows.map((row) => row.location)).size || "—" },
      ]}
    />
  );
}
