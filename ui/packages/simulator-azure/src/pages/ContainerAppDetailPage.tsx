import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials, AzureStatus } from "../portal/AzurePortal.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import {
  fetchContainerAppJob,
  fetchContainerAppJobExecutions,
  startContainerAppJobExecution,
  stopContainerAppJobExecutions,
} from "../api.js";

// The Container Apps blade this console lists (ContainerAppsPage) reads the
// real Microsoft.App/jobs resource — Container Apps' run-to-completion
// model, the one sockerless deploys container tasks onto — so this detail
// blade stays on that same resource: its real Essentials, its containers
// (from the job's own template), and its run history (the real executions
// list), all read and driven through real Azure Resource Manager calls.
export function ContainerAppDetailPage() {
  const { name = "" } = useParams();
  const queryClient = useQueryClient();

  const job = useQuery({ queryKey: ["ca-job", name], queryFn: () => fetchContainerAppJob(name) });
  const jobId = job.data?.id;
  const executions = useQuery({
    queryKey: ["ca-job-executions", jobId],
    queryFn: () => fetchContainerAppJobExecutions(jobId!),
    enabled: Boolean(jobId),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["ca-job-executions", jobId] });
  };
  const start = useMutation({
    mutationFn: () => startContainerAppJobExecution(jobId!),
    onSuccess: invalidate,
  });
  const stop = useMutation({
    mutationFn: () => stopContainerAppJobExecutions(jobId!),
    onSuccess: invalidate,
  });

  const mutationError = start.error ?? stop.error;

  return (
    <>
      <AzureCommandBar
        commands={[
          {
            label: "Run now",
            icon: "play",
            testid: "ca-job-run",
            disabled: !jobId || start.isPending,
            onSelect: () => start.mutate(),
          },
          {
            label: "Stop all executions",
            icon: "stop",
            testid: "ca-job-stop",
            disabled: !jobId || stop.isPending,
            onSelect: () => stop.mutate(),
          },
          {
            label: "Refresh",
            icon: "refresh",
            onSelect: () => {
              void job.refetch();
              void executions.refetch();
            },
            disabled: job.isFetching,
          },
        ]}
      />
      <div className="az-main" data-testid="ca-job-detail">
        {job.isError ? (
          <div className="az-message az-message-error" role="alert" data-testid="ca-job-error">
            <strong>Could not load this Container Apps job.</strong>{" "}
            {job.error instanceof Error ? job.error.message : "Azure Resource Manager did not respond."}
          </div>
        ) : job.isLoading || !job.data ? (
          <div className="az-empty" role="status">
            Loading the job…
          </div>
        ) : (
          <>
            <AzureEssentials
              properties={[
                { label: "Resource group", value: resourceGroupOf(job.data.id) },
                { label: "Location", value: locationLabel(job.data.location) },
                { label: "Provisioning state", value: <AzureStatus status={job.data.provisioningState || "Unknown"} /> },
                { label: "Trigger type", value: job.data.triggerType || "—" },
                { label: "Replica timeout", value: job.data.replicaTimeout ? `${job.data.replicaTimeout}s` : "—" },
                {
                  label: "Environment",
                  value: job.data.environmentId ? (job.data.environmentId.split("/").pop() ?? "—") : "—",
                },
              ]}
            />

            {mutationError ? (
              <div className="az-message az-message-error" role="alert" data-testid="ca-job-action-error">
                <strong>The job operation failed.</strong>{" "}
                {mutationError instanceof Error ? mutationError.message : "Azure Resource Manager did not respond."}
              </div>
            ) : null}

            <section className="az-blade-section" aria-label="Containers">
              <h2>Containers</h2>
              <table className="az-table" data-testid="ca-job-containers">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Image</th>
                    <th>Command</th>
                  </tr>
                </thead>
                <tbody>
                  {job.data.containers.length === 0 ? (
                    <tr>
                      <td className="az-table-state" colSpan={3}>
                        <div className="az-empty">
                          <strong>This job's template has no containers.</strong>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    job.data.containers.map((container) => (
                      <tr key={container.name}>
                        <td>{container.name}</td>
                        <td>
                          <code>{container.image}</code>
                        </td>
                        <td>
                          <code>{[...container.command, ...container.args].join(" ") || "—"}</code>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </section>

            <section className="az-blade-section" aria-label="Execution history">
              <h2>Execution history</h2>
              <table className="az-table" data-testid="ca-job-executions">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Status</th>
                    <th>Start time</th>
                    <th>End time</th>
                  </tr>
                </thead>
                <tbody>
                  {executions.isError ? (
                    <tr>
                      <td className="az-table-state" colSpan={4}>
                        <div className="az-message az-message-error" role="alert">
                          <strong>Could not load executions.</strong>{" "}
                          {executions.error instanceof Error ? executions.error.message : "Azure Resource Manager did not respond."}
                        </div>
                      </td>
                    </tr>
                  ) : executions.isLoading ? (
                    <tr>
                      <td className="az-table-state" colSpan={4}>
                        <div className="az-empty" role="status">
                          Loading executions…
                        </div>
                      </td>
                    </tr>
                  ) : (executions.data ?? []).length === 0 ? (
                    <tr>
                      <td className="az-table-state" colSpan={4}>
                        <div className="az-empty">
                          <strong>No executions yet</strong>
                          <p>Runs of this job appear here.</p>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    (executions.data ?? []).map((execution) => (
                      <tr key={execution.name}>
                        <td>{execution.name}</td>
                        <td>
                          <AzureStatus status={execution.status || "Unknown"} />
                        </td>
                        <td>{execution.startTime || "—"}</td>
                        <td>{execution.endTime || "—"}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </section>
          </>
        )}
      </div>
    </>
  );
}
