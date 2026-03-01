import { useMetrics, useStatus } from "../hooks/index.js";
import { MetricsCard } from "../components/MetricsCard.js";
import { Spinner } from "../components/Spinner.js";
import { RefreshButton } from "../components/RefreshButton.js";

export function MetricsPage() {
  const { data: metrics, isLoading, refetch, isFetching } = useMetrics();
  const { data: status } = useStatus();

  if (isLoading) return <Spinner />;

  const requestEntries = metrics ? Object.entries(metrics.requests).sort((a, b) => b[1] - a[1]) : [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Metrics</h2>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Goroutines" value={metrics?.goroutines ?? 0} />
        <MetricsCard
          title="Heap"
          value={`${(metrics?.heap_alloc_mb ?? 0).toFixed(1)} MB`}
        />
        <MetricsCard title="Containers" value={status?.containers ?? 0} />
        <MetricsCard title="Active Resources" value={status?.active_resources ?? 0} />
      </div>

      {requestEntries.length > 0 && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Endpoint
                </th>
                <th className="px-4 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  Count
                </th>
                <th className="px-4 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  P50 (ms)
                </th>
                <th className="px-4 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  P95 (ms)
                </th>
                <th className="px-4 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  P99 (ms)
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-900">
              {requestEntries.map(([endpoint, count]) => {
                const lat = metrics?.latency_ms[endpoint];
                return (
                  <tr key={endpoint} className="hover:bg-gray-50 dark:hover:bg-gray-800">
                    <td className="whitespace-nowrap px-4 py-2 text-sm font-mono">{endpoint}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm text-right">{count}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm text-right">{lat?.p50 ?? "—"}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm text-right">{lat?.p95 ?? "—"}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-sm text-right">{lat?.p99 ?? "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
