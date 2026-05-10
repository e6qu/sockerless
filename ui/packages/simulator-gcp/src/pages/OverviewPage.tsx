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
        kicker="gcp · simulator"
        title={<>Overview</>}
        meta="Service-by-service resource counts."
        actions={<StatusBadge status={isOk ? "running" : "error"} />}
      />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard
          title="Cloud Run Jobs"
          value={services.cloudrun_jobs ?? 0}
        />
        <MetricsCard
          title="Cloud Functions"
          value={services.functions ?? 0}
        />
        <MetricsCard
          title="AR Repositories"
          value={services.ar_repos ?? 0}
        />
        <MetricsCard title="GCS Buckets" value={services.gcs_buckets ?? 0} />
        <MetricsCard title="Log Entries" value={services.log_entries ?? 0} />
      </div>
    </div>
  );
}
