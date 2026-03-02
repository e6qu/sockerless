import { useQuery } from "@tanstack/react-query";
import { MetricsCard, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";

const api = new AdminApiClient();

export function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["overview"],
    queryFn: () => api.overview(),
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">System Overview</h2>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Components Up" value={data.components_up} />
        <MetricsCard title="Components Down" value={data.components_down} />
        <MetricsCard title="Total Backends" value={data.backends} />
        <MetricsCard title="Total Containers" value={data.total_containers} />
      </div>

      <h3 className="text-lg font-medium">Component Health</h3>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {data.components.map((c) => (
          <div
            key={c.name}
            className="flex items-center justify-between rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
          >
            <div>
              <p className="font-medium">{c.name}</p>
              <p className="text-xs text-gray-500 dark:text-gray-400">{c.type} &middot; {c.addr}</p>
            </div>
            <StatusBadge status={c.health === "up" ? "ok" : "error"} />
          </div>
        ))}
      </div>
    </div>
  );
}
