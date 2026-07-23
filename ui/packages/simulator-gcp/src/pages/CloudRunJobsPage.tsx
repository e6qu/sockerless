import { Link } from "react-router";
import { GcpResourceTable, GcpStatus, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudRunJobsReal, type CloudRunJob } from "../api.js";

// The real Job resource reports readiness through its terminal condition; the
// console reads that rather than inventing a status field.
function jobState(job: CloudRunJob): string {
  const condition = job.terminalCondition;
  if (!condition) return job.reconciling ? "Reconciling" : "Unknown";
  if (condition.state === "CONDITION_SUCCEEDED") return "Ready";
  if (condition.state === "CONDITION_FAILED") return condition.message ? `Failed: ${condition.message}` : "Failed";
  return condition.state ?? "Unknown";
}

const columns: GcpColumn<CloudRunJob>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/cloudrun/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "state", header: "Status of last execution", cell: (row) => <GcpStatus status={jobState(row)} />, value: jobState },
  { id: "created", header: "Created", cell: (row) => formatTimestamp(row.createTime ?? ""), value: (row) => row.createTime ?? "" },
  { id: "executions", header: "Executions", cell: (row) => row.executionCount ?? 0, value: (row) => String(row.executionCount ?? 0) },
  { id: "launchStage", header: "Launch stage", cell: (row) => row.launchStage ?? "—", value: (row) => row.launchStage ?? "" },
];

export function CloudRunJobsPage() {
  return (
    <GcpResourceTable<CloudRunJob>
      title="Cloud Run jobs"
      description="A job executes tasks to completion. Jobs are ideal for batch processing and scheduled workloads."
      actions={[{ label: "Create job", icon: "add", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["cloudrun-jobs-real"]}
      queryFn={fetchCloudRunJobsReal}
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
