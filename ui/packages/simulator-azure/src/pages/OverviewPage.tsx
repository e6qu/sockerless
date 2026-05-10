import { useSimSummary, useSimHealth } from "@sockerless/ui-core/hooks";
import {
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";

export function OverviewPage() {
  const health = useSimHealth();
  const summary = useSimSummary();

  if (health.isLoading || summary.isLoading) return <Spinner label="loading" />;

  const services = summary.data?.services ?? {};
  const isOk = health.data?.status === "ok";

  return (
    <div className="flex flex-col gap-6">
      <PageHeading
        kicker="azure · simulator"
        title={<>Overview</>}
        meta="Service-by-service resource counts."
        actions={<StatusBadge status={isOk ? "running" : "error"} />}
      />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard
          title="Container App Jobs"
          value={services.container_app_jobs ?? 0}
        />
        <MetricsCard
          title="Function Sites"
          value={services.function_sites ?? 0}
        />
        <MetricsCard
          title="ACR Registries"
          value={services.acr_registries ?? 0}
        />
        <MetricsCard
          title="Storage Accounts"
          value={services.storage_accounts ?? 0}
        />
        <MetricsCard title="Monitor Logs" value={services.monitor_logs ?? 0} />
      </div>
    </div>
  );
}
