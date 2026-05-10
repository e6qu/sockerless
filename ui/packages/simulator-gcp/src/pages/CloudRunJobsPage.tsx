import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchCloudRunJobs, type CloudRunJob } from "../api.js";

const columns: ColumnDef<CloudRunJob, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "createTime", header: "Created" },
  { accessorKey: "executionCount", header: "Executions", sortDescFirst: true },
  { accessorKey: "launchStage", header: "Launch Stage" },
];

export function CloudRunJobsPage() {
  return (
    <ResourceListPage<CloudRunJob>
      kicker="gcp · simulator · cloudrun"
      title={<>Jobs</>}
      countNoun="job"
      columns={columns}
      queryKey={["cloudrun-jobs"]}
      queryFn={fetchCloudRunJobs}
      filterPlaceholder="Filter jobs…"
      emptyMessage="No Cloud Run jobs tracked."
    />
  );
}
