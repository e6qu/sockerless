import { useQuery } from "@tanstack/react-query";
import { MetricsCard, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { fetchHealth, fetchMetrics, fetchWorkflows } from "../api.js";
import { useNavigate } from "react-router";
import type { BleephubWorkflow } from "../types.js";

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function OverviewPage() {
  const navigate = useNavigate();
  const { data: health } = useQuery({ queryKey: ["health"], queryFn: fetchHealth });
  const { data: metrics, isLoading } = useQuery({ queryKey: ["metrics"], queryFn: fetchMetrics });
  const { data: workflows } = useQuery({ queryKey: ["workflows"], queryFn: fetchWorkflows });

  if (isLoading || !metrics) return <Spinner />;

  const recent = (workflows ?? []).slice(0, 10);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-semibold">Overview</h2>
        {health && <StatusBadge status={health.status === "ok" ? "ok" : "error"} />}
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
        <MetricsCard title="Active Workflows" value={metrics.active_workflows} />
        <MetricsCard title="Connected Runners" value={metrics.active_sessions} />
        <MetricsCard title="Submissions" value={metrics.workflow_submissions} />
        <MetricsCard title="Job Dispatches" value={metrics.job_dispatches} />
        <MetricsCard title="Uptime" value={formatUptime(metrics.uptime_seconds)} />
      </div>

      {recent.length > 0 && (
        <>
          <h3 className="text-lg font-medium">Recent Workflows</h3>
          <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Name</th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Status</th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Result</th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Event</th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Jobs</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 bg-white dark:divide-gray-700 dark:bg-gray-900">
                {recent.map((wf: BleephubWorkflow) => (
                  <tr
                    key={wf.id}
                    className="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800"
                    onClick={() => navigate(`/ui/workflows/${wf.id}`)}
                  >
                    <td className="whitespace-nowrap px-4 py-2 text-sm font-medium">{wf.name}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm"><StatusBadge status={wf.status} /></td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm">{wf.result && <StatusBadge status={wf.result} />}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-500">{wf.eventName}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm">{Object.keys(wf.jobs).length}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
