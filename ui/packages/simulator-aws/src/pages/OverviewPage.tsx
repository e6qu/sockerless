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
        kicker="aws · simulator"
        title={<>Overview</>}
        meta="Service-by-service resource counts."
        actions={<StatusBadge status={isOk ? "running" : "error"} />}
      />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <MetricsCard title="ECS Tasks" value={services.ecs_tasks ?? 0} />
        <MetricsCard
          title="Lambda Functions"
          value={services.lambda_functions ?? 0}
        />
        <MetricsCard
          title="ECR Repositories"
          value={services.ecr_repositories ?? 0}
        />
        <MetricsCard title="S3 Buckets" value={services.s3_buckets ?? 0} />
        <MetricsCard title="Log Groups" value={services.cw_log_groups ?? 0} />
      </div>
    </div>
  );
}
