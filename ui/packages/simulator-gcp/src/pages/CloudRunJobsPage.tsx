import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { GcpResourceTable, GcpStatus, type GcpColumn } from "../console/index.js";
import { GcpDialog } from "../console/GcpDialog.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { deleteCloudRunJob, fetchCloudRunJobsReal, waitV2Operation, type CloudRunJob } from "../api.js";
import { useProject } from "../console/project.js";

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

// DeleteJobDialog is shared by the list's per-row action and the job detail
// page's header action — the same real projects.locations.jobs.delete
// long-running operation, driven through the same operations.get poll
// (waitV2Operation) Cloud Functions deletion uses.
export function DeleteJobDialog({
  project,
  jobId,
  onClose,
  onDeleted,
}: {
  project: string;
  jobId: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const remove = useMutation({
    mutationFn: async () => waitV2Operation(await deleteCloudRunJob(project, jobId)),
    onSuccess: onDeleted,
  });
  return (
    <GcpDialog title="Delete job?" testId="cloudrun-delete-dialog" onClose={onClose}>
      <p>
        Deleting <strong>{jobId}</strong> permanently removes the job and its execution history.
        This can't be undone.
      </p>
      {remove.isError ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't delete the job.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The API did not respond."}
        </div>
      ) : null}
      <div className="gc-dialog-actions">
        <button type="button" className="gc-button-text" onClick={onClose}>Cancel</button>
        <button
          type="button"
          className="gc-button-primary"
          data-testid="cloudrun-delete-confirm"
          disabled={remove.isPending}
          onClick={() => remove.mutate()}
        >
          {remove.isPending ? "Deleting…" : "Delete"}
        </button>
      </div>
    </GcpDialog>
  );
}

export function CloudRunJobsPage() {
  const { project } = useProject();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState<string | null>(null);

  const columnsWithActions: GcpColumn<CloudRunJob>[] = [
    ...columns,
    {
      id: "actions",
      header: "Actions",
      cell: (row) => {
        const id = shortName(row.name);
        return (
          <button
            type="button"
            className="gc-button-text"
            data-testid={`cloudrun-delete-${id}`}
            aria-label={`Delete ${id}`}
            onClick={() => setDeleting(id)}
          >
            Delete
          </button>
        );
      },
      value: () => "",
    },
  ];

  return (
    <>
      <GcpResourceTable<CloudRunJob>
        title="Cloud Run jobs"
        description="A job executes tasks to completion. Jobs are ideal for batch processing and scheduled workloads."
        actions={[{ label: "Create job", icon: "add", primary: true, disabled: true }]}
        columns={columnsWithActions}
        queryKey={["cloudrun-jobs-real", project]}
        queryFn={() => fetchCloudRunJobsReal(project)}
        filterPlaceholder="Filter jobs"
        resourceNoun="jobs"
        empty={{
          headline: "Execute jobs on a fully managed platform",
          description: "Cloud Run jobs execute containers to completion.",
          primaryLabel: "Create job",
        }}
        rowKey={(row) => row.name}
      />
      {deleting ? (
        <DeleteJobDialog
          project={project}
          jobId={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            void queryClient.invalidateQueries({ queryKey: ["cloudrun-jobs-real", project] });
          }}
        />
      ) : null}
    </>
  );
}
