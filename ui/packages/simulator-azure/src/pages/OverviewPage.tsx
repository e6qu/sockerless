import { useSimSummary, useSimHealth } from "@sockerless/ui-core/hooks";
import { MetricsCard, Spinner, StatusBadge } from "@sockerless/ui-core/components";

export function OverviewPage() {
  const health = useSimHealth();
  const summary = useSimSummary();

  if (health.isLoading || summary.isLoading) return <Spinner />;

  const services = summary.data?.services ?? {};
  return (
    <div>
      <div className="mb-6 flex items-center gap-3">
        <h2 className="text-2xl font-bold">Azure Simulator</h2>
        {health.data && <StatusBadge status={health.data.status === "ok" ? "running" : "error"} />}
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard label="Container App Jobs" value={services.container_app_jobs ?? 0} />
        <MetricsCard label="Function Sites" value={services.function_sites ?? 0} />
        <MetricsCard label="ACR Registries" value={services.acr_registries ?? 0} />
        <MetricsCard label="Storage Accounts" value={services.storage_accounts ?? 0} />
        <MetricsCard label="Monitor Logs" value={services.monitor_logs ?? 0} />
      </div>
    </div>
  );
}
