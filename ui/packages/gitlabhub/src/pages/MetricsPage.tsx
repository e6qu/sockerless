import { useQuery } from "@tanstack/react-query";
import { MetricsCard, Spinner } from "@sockerless/ui-core/components";
import { fetchMetrics, fetchStatus } from "../api.js";

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function MetricsPage() {
  const { data: metrics, isLoading: metricsLoading } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5000,
  });
  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["status"],
    queryFn: fetchStatus,
    refetchInterval: 5000,
  });

  if ((metricsLoading && !metrics) || (statusLoading && !status)) return <Spinner />;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">Metrics</h2>

      {metrics && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <MetricsCard title="Pipeline Submissions" value={metrics.pipeline_submissions} />
          <MetricsCard title="Job Dispatches" value={metrics.job_dispatches} />
          <MetricsCard title="Active Pipelines" value={metrics.active_pipelines} />
          <MetricsCard title="Registered Runners" value={metrics.registered_runners} />
          <MetricsCard title="Goroutines" value={metrics.goroutines} />
          <MetricsCard title="Heap (MB)" value={metrics.heap_alloc_mb.toFixed(1)} />
          <MetricsCard title="Uptime" value={formatUptime(metrics.uptime_seconds)} />
        </div>
      )}

      {status && (
        <>
          <h3 className="text-lg font-medium">Jobs by Status</h3>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            {Object.entries(status.jobs_by_status).map(([status, count]) => (
              <MetricsCard key={status} title={status} value={count} />
            ))}
          </div>
        </>
      )}

      {metrics && Object.keys(metrics.job_completions).length > 0 && (
        <>
          <h3 className="text-lg font-medium">Job Completions</h3>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            {Object.entries(metrics.job_completions).map(([result, count]) => (
              <MetricsCard key={result} title={result} value={count} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
