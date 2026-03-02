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
        <h2 className="text-2xl font-bold">GCP Simulator</h2>
        {health.data && <StatusBadge status={health.data.status === "ok" ? "running" : "error"} />}
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard label="Cloud Run Jobs" value={services.cloudrun_jobs ?? 0} />
        <MetricsCard label="Cloud Functions" value={services.functions ?? 0} />
        <MetricsCard label="AR Repositories" value={services.ar_repos ?? 0} />
        <MetricsCard label="GCS Buckets" value={services.gcs_buckets ?? 0} />
        <MetricsCard label="Log Entries" value={services.log_entries ?? 0} />
      </div>
    </div>
  );
}
