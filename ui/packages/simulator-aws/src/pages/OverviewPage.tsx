import { Link } from "react-router";
import { useSimSummary, useSimHealth } from "@sockerless/ui-core/hooks";
import { AwsContainer, AwsPageHeader, AwsStatus } from "../console/index.js";

/**
 * The AWS console's service dashboard states where you are looking before it
 * states what is there: Region and service health, then counts that link
 * through to the resource that owns them.
 */
const SERVICES: { label: string; key: string; to: string }[] = [
  { label: "Tasks", key: "ecs_tasks", to: "/ui/ecs" },
  { label: "Functions", key: "lambda_functions", to: "/ui/lambda" },
  { label: "Repositories", key: "ecr_repositories", to: "/ui/ecr" },
  { label: "Buckets", key: "s3_buckets", to: "/ui/s3" },
  { label: "Log groups", key: "cw_log_groups", to: "/ui/logs" },
];

export function OverviewPage() {
  const health = useSimHealth();
  const summary = useSimSummary();

  if (health.isLoading || summary.isLoading) {
    return (
      <>
        <AwsPageHeader title="Overview" />
        <AwsContainer>
          <div className="aws-empty">Loading service overview…</div>
        </AwsContainer>
      </>
    );
  }

  // A failed fetch must not render an all-zeros board that reads as healthy.
  if (health.isError || summary.isError) {
    return (
      <>
        <AwsPageHeader title="Overview" />
        <AwsContainer>
          <div className="aws-flash aws-flash-error" role="alert">
            <strong>Could not load the service overview.</strong>{" "}
            {String(summary.error ?? health.error)}
          </div>
        </AwsContainer>
      </>
    );
  }

  const services = (summary.data?.services ?? {}) as Record<string, number>;
  const isOk = health.data?.status === "ok";

  return (
    <>
      <AwsPageHeader title="Overview" description="Resources in this account and Region." />
      <div className="aws-overview-grid">
        <AwsContainer>
          <div className="aws-metric">
            <div className="aws-metric-label">Region</div>
            <div style={{ marginTop: 4 }}>eu-west-1</div>
            <div className="aws-metric-label" style={{ marginTop: 12 }}>Service health</div>
            <div style={{ marginTop: 4 }}>
              <AwsStatus status={isOk ? "Operating normally" : "Unavailable"} kind={isOk ? "success" : "error"} />
            </div>
          </div>
        </AwsContainer>
        {SERVICES.map((service) => (
          <AwsContainer key={service.key}>
            <div className="aws-metric">
              <div className="aws-metric-label">{service.label}</div>
              <div className="aws-metric-value">
                <Link to={service.to}>{services[service.key] ?? 0}</Link>
              </div>
            </div>
          </AwsContainer>
        ))}
      </div>
    </>
  );
}
