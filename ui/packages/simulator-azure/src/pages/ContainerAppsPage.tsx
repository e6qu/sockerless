import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchContainerAppJobs, type ContainerAppJob } from "../api.js";

const columns: ColumnDef<ContainerAppJob, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "type", header: "Type" },
  { accessorKey: "id", header: "Resource ID" },
];

export function ContainerAppsPage() {
  return (
    <ResourceListPage<ContainerAppJob>
      kicker="azure · simulator · container apps"
      title={<>Jobs</>}
      countNoun="job"
      columns={columns}
      queryKey={["ca-jobs"]}
      queryFn={fetchContainerAppJobs}
      filterPlaceholder="Filter jobs…"
      emptyMessage="No Container App jobs tracked."
    />
  );
}
