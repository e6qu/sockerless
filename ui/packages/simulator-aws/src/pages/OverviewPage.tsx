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
        <h2 className="text-2xl font-bold">AWS Simulator</h2>
        {health.data && <StatusBadge status={health.data.status === "ok" ? "running" : "error"} />}
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard label="ECS Tasks" value={services.ecs_tasks ?? 0} />
        <MetricsCard label="Lambda Functions" value={services.lambda_functions ?? 0} />
        <MetricsCard label="ECR Repositories" value={services.ecr_repositories ?? 0} />
        <MetricsCard label="S3 Buckets" value={services.s3_buckets ?? 0} />
        <MetricsCard label="Log Groups" value={services.cw_log_groups ?? 0} />
      </div>
    </div>
  );
}
