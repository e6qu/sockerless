import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudRunJobs, type CloudRunJob } from "../api.js";

const columns: GcpColumn<CloudRunJob>[] = [
  { id: "name", header: "Name", cell: (row) => shortName(row.name), value: (row) => shortName(row.name) },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime), value: (row) => row.createTime },
  { id: "executionCount", header: "Executions", cell: (row) => row.executionCount, value: (row) => String(row.executionCount) },
  { id: "launchStage", header: "Launch stage", cell: (row) => row.launchStage, value: (row) => row.launchStage },
];

export function CloudRunJobsPage() {
  return (
    <GcpResourceTable<CloudRunJob>
      title="Cloud Run jobs"
      description="A job executes tasks to completion. Jobs are ideal for batch processing and scheduled workloads."
      actions={[{ label: "Create job", glyph: "+", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["cloudrun-jobs"]}
      queryFn={fetchCloudRunJobs}
      filterPlaceholder="Filter jobs"
      resourceNoun="jobs"
      empty={{
        headline: "Execute jobs on a fully managed platform",
        description: "Cloud Run jobs execute containers to completion.",
        primaryLabel: "Create job",
      }}
      rowKey={(row) => row.name}
    />
  );
}
