import { useQuery } from "@tanstack/react-query";
import { MetricsCard, StatusBadge, Spinner, AppShell, type NavItem } from "@sockerless/ui-core/components";
import { fetchHealth, fetchStatus, fetchMetrics } from "./api.js";

const navItems: NavItem[] = [
  { label: "Overview", to: "/ui/" },
];

function renderLink(item: NavItem) {
  return (
    <span className="block rounded-md px-3 py-2 text-sm font-medium bg-blue-50 text-blue-700 dark:bg-blue-900 dark:text-blue-200">
      {item.label}
    </span>
  );
}

export function App() {
  const { data: health } = useQuery({
    queryKey: ["frontend-health"],
    queryFn: fetchHealth,
    refetchInterval: 10_000,
  });
  const { data: status, isLoading } = useQuery({
    queryKey: ["frontend-status"],
    queryFn: fetchStatus,
    refetchInterval: 5_000,
  });
  const { data: metrics } = useQuery({
    queryKey: ["frontend-metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5_000,
  });

  return (
    <AppShell title="Docker Frontend" navItems={navItems} renderLink={renderLink}>
      {isLoading ? (
        <Spinner />
      ) : (
        <div className="space-y-6">
          <h2 className="text-xl font-semibold">Frontend Overview</h2>

          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <MetricsCard
              title="Docker Requests"
              value={metrics?.docker_requests ?? 0}
            />
            <MetricsCard title="Goroutines" value={metrics?.goroutines ?? 0} />
            <MetricsCard
              title="Heap"
              value={`${(metrics?.heap_alloc_mb ?? 0).toFixed(1)} MB`}
            />
            <MetricsCard
              title="Uptime"
              value={formatUptime(status?.uptime_seconds ?? 0)}
            />
          </div>

          {status && (
            <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
              <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-3">Configuration</h3>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                <dt className="text-gray-500 dark:text-gray-400">Docker Address</dt>
                <dd className="font-mono text-xs">{status.docker_addr}</dd>
                <dt className="text-gray-500 dark:text-gray-400">Backend Address</dt>
                <dd className="font-mono text-xs">{status.backend_addr}</dd>
              </dl>
            </div>
          )}

          {health && (
            <p className="text-xs text-gray-400">
              Health: <StatusBadge status={health.status} /> | Component: {health.component}
            </p>
          )}
        </div>
      )}
    </AppShell>
  );
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}
