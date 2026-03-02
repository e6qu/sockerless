import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { StatusBadge, Spinner, LogViewer } from "@sockerless/ui-core/components";
import { fetchWorkflowDetail, fetchWorkflowLogs } from "../api.js";
import type { BleephubWorkflowJob } from "../types.js";

export function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: wf, isLoading } = useQuery({
    queryKey: ["workflow", id],
    queryFn: () => fetchWorkflowDetail(id!),
    enabled: !!id,
    refetchInterval: 3000,
  });
  const { data: logs } = useQuery({
    queryKey: ["workflow-logs", id],
    queryFn: () => fetchWorkflowLogs(id!),
    enabled: !!id,
    refetchInterval: 5000,
  });

  if (isLoading || !wf) return <Spinner />;

  const jobs = Object.values(wf.jobs).sort((a, b) => a.key.localeCompare(b.key));

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{wf.name}</h2>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
          <span>Run #{wf.runId}</span>
          <StatusBadge status={wf.status} />
          {wf.result && <StatusBadge status={wf.result} />}
          {wf.eventName && <span>Event: {wf.eventName}</span>}
          {wf.repoFullName && <span>Repo: {wf.repoFullName}</span>}
          <span>{new Date(wf.createdAt).toLocaleString()}</span>
        </div>
      </div>

      <h3 className="text-lg font-medium">Jobs ({jobs.length})</h3>
      <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead className="bg-gray-50 dark:bg-gray-800">
            <tr>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Key</th>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Name</th>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Status</th>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Result</th>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Needs</th>
              <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Matrix</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-900">
            {jobs.map((job: BleephubWorkflowJob) => (
              <tr key={job.key} className="hover:bg-gray-50 dark:hover:bg-gray-800">
                <td className="whitespace-nowrap px-4 py-2 text-sm font-mono">{job.key}</td>
                <td className="whitespace-nowrap px-4 py-2 text-sm">{job.displayName}</td>
                <td className="whitespace-nowrap px-4 py-2 text-sm"><StatusBadge status={job.status} /></td>
                <td className="whitespace-nowrap px-4 py-2 text-sm">{job.result && <StatusBadge status={job.result} />}</td>
                <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-500">{job.needs?.join(", ")}</td>
                <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-500">
                  {job.matrix && Object.entries(job.matrix).map(([k, v]) => `${k}=${v}`).join(", ")}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {logs && Object.keys(logs).length > 0 && (
        <div className="space-y-4">
          <h3 className="text-lg font-medium">Logs</h3>
          {jobs.map((job) => {
            const jobLogs = logs[job.jobId];
            if (!jobLogs || jobLogs.length === 0) return null;
            return (
              <div key={job.jobId}>
                <p className="mb-1 text-sm font-medium">{job.displayName}</p>
                <LogViewer lines={jobLogs} />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
