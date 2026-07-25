import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { GcpPageHeader, GcpStatus } from "../console/GcpConsole.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudRunJob, fetchCloudRunJobExecutions } from "../api.js";
import { useProject } from "../console/project.js";

export function CloudRunJobDetailPage() {
  const { name = "" } = useParams();
  const { project } = useProject();
  const job = useQuery({ queryKey: ["cloudrun-job", project, name], queryFn: () => fetchCloudRunJob(project, name) });
  const executions = useQuery({
    queryKey: ["cloudrun-job-executions", project, name],
    queryFn: () => fetchCloudRunJobExecutions(project, name),
    enabled: Boolean(job.data),
  });

  if (job.isError) {
    return (
      <>
        <GcpPageHeader title={name} description="Cloud Run job" />
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't load this job.</strong>{" "}
          {job.error instanceof Error ? job.error.message : "The simulator did not respond."}
        </div>
      </>
    );
  }

  const data = job.data;
  const condition = data?.terminalCondition;
  const status = !condition
    ? "Unknown"
    : condition.state === "CONDITION_SUCCEEDED"
      ? "Ready"
      : condition.state === "CONDITION_FAILED"
        ? "Failed"
        : condition.state ?? "Unknown";

  const properties: { label: string; value: React.ReactNode }[] = data
    ? [
        { label: "Status", value: <GcpStatus status={status} /> },
        { label: "Unique ID", value: data.uid ?? "—" },
        { label: "Executions", value: String(data.executionCount ?? 0) },
        { label: "Launch stage", value: data.launchStage ?? "—" },
        { label: "Created", value: formatTimestamp(data.createTime ?? "") },
        { label: "Updated", value: formatTimestamp(data.updateTime ?? "") },
        { label: "Reconciling", value: data.reconciling ? "Yes" : "No" },
      ]
    : [];
  const labels = Object.entries(data?.labels ?? {});

  return (
    <>
      <div className="gc-detail-back">
        <Link to="/ui/cloudrun">‹ Cloud Run jobs</Link>
      </div>
      <GcpPageHeader
        title={shortName(name)}
        description="Cloud Run job"
        onRefresh={() => { void job.refetch(); void executions.refetch(); }}
        refreshing={job.isFetching || executions.isFetching}
      />

      {job.isLoading ? (
        <div className="gc-loading" role="status">Loading job…</div>
      ) : (
        <>
          <dl className="gc-detail-grid">
            {properties.map((property) => (
              <div className="gc-detail-pair" key={property.label}>
                <dt>{property.label}</dt>
                <dd>{property.value}</dd>
              </div>
            ))}
          </dl>

          {labels.length > 0 ? (
            <>
              <h2 className="gc-detail-heading">Labels</h2>
              <dl className="gc-detail-grid">
                {labels.map(([key, value]) => (
                  <div className="gc-detail-pair" key={key}>
                    <dt>{key}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </>
          ) : null}

          <h2 className="gc-detail-heading">Executions</h2>
          <div className="gc-table-wrap">
            <table className="gc-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Created</th>
                  <th>Completed</th>
                  <th>Succeeded</th>
                  <th>Failed</th>
                </tr>
              </thead>
              <tbody>
                {(executions.data ?? []).length === 0 ? (
                  <tr>
                    <td className="gc-table-state" colSpan={5}>
                      <div className="gc-empty">
                        <p className="gc-empty-headline">No executions yet</p>
                        <p className="gc-empty-description">Runs of this job appear here.</p>
                      </div>
                    </td>
                  </tr>
                ) : (
                  (executions.data ?? []).map((execution) => (
                    <tr key={execution.name}>
                      <td>{shortName(execution.name)}</td>
                      <td>{formatTimestamp(execution.createTime ?? "")}</td>
                      <td>{execution.completionTime ? formatTimestamp(execution.completionTime) : "—"}</td>
                      <td>{execution.succeededCount ?? 0}</td>
                      <td>{execution.failedCount ?? 0}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </>
  );
}
