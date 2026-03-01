import { useStatus, useHealth, useInfo, useCheck } from "../hooks/index.js";
import { MetricsCard } from "../components/MetricsCard.js";
import { StatusBadge } from "../components/StatusBadge.js";
import { Spinner } from "../components/Spinner.js";
import { BackendInfoCard } from "../components/BackendInfoCard.js";

export function OverviewPage() {
  const { data: status, isLoading: statusLoading } = useStatus();
  const { data: health } = useHealth();
  const { data: info } = useInfo();
  const { data: checks } = useCheck();

  if (statusLoading) return <Spinner />;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">Overview</h2>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Containers" value={status?.containers ?? 0} />
        <MetricsCard title="Active Resources" value={status?.active_resources ?? 0} />
        <MetricsCard
          title="Uptime"
          value={formatUptime(status?.uptime_seconds ?? 0)}
        />
        <MetricsCard title="Backend" value={status?.backend_type ?? "—"} />
      </div>

      {status && <BackendInfoCard status={status} />}

      {info && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-3">System Info</h3>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <dt className="text-gray-500 dark:text-gray-400">Name</dt>
            <dd>{info.Name}</dd>
            <dt className="text-gray-500 dark:text-gray-400">Version</dt>
            <dd>{info.ServerVersion}</dd>
            <dt className="text-gray-500 dark:text-gray-400">Driver</dt>
            <dd>{info.Driver}</dd>
            <dt className="text-gray-500 dark:text-gray-400">OS / Arch</dt>
            <dd>{info.OperatingSystem} ({info.Architecture})</dd>
            <dt className="text-gray-500 dark:text-gray-400">Images</dt>
            <dd>{info.Images}</dd>
          </dl>
        </div>
      )}

      {checks && checks.checks.length > 0 && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-3">Health Checks</h3>
          <ul className="space-y-2">
            {checks.checks.map((c) => (
              <li key={c.name} className="flex items-center gap-2 text-sm">
                <StatusBadge status={c.status} />
                <span className="font-medium">{c.name}</span>
                {c.detail && <span className="text-gray-400">— {c.detail}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}

      {health && (
        <p className="text-xs text-gray-400">
          Health: {health.status} | Component: {health.component}
        </p>
      )}
    </div>
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
