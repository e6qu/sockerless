import { Link } from "react-router";
import { useQueries } from "@tanstack/react-query";
import { AwsContainer, AwsPageHeader, AwsStatus } from "../console/index.js";
import {
  fetchECSTasks,
  fetchLambdaFunctions,
  fetchECRRepos,
  fetchS3Buckets,
  fetchCWLogGroups,
} from "../api.js";

/**
 * The AWS console's service dashboard states where you are looking before it
 * states what is there: Region and service health, then counts that link
 * through to the resource that owns them. Each count comes from the same real
 * API its resource page reads, not a sockerless-invented summary.
 */
const SERVICES = [
  { label: "Tasks", to: "/ui/ecs", queryKey: ["ecs-tasks"], queryFn: fetchECSTasks },
  { label: "Functions", to: "/ui/lambda", queryKey: ["lambda-functions"], queryFn: fetchLambdaFunctions },
  { label: "Repositories", to: "/ui/ecr", queryKey: ["ecr-repos"], queryFn: fetchECRRepos },
  { label: "Buckets", to: "/ui/s3", queryKey: ["s3-buckets"], queryFn: fetchS3Buckets },
  { label: "Log groups", to: "/ui/logs", queryKey: ["cw-log-groups"], queryFn: fetchCWLogGroups },
] as const;

export function OverviewPage() {
  const results = useQueries({
    queries: SERVICES.map((service) => ({ queryKey: service.queryKey, queryFn: service.queryFn })),
  });
  const failed = results.some((result) => result.isError);
  const loading = results.some((result) => result.isLoading);

  if (loading) {
    return (
      <>
        <AwsPageHeader title="Overview" />
        <AwsContainer>
          <div className="aws-empty">Loading service overview…</div>
        </AwsContainer>
      </>
    );
  }

  // A failed read must not render an all-zeros board that reads as healthy.
  if (failed) {
    return (
      <>
        <AwsPageHeader title="Overview" />
        <AwsContainer>
          <div className="aws-flash aws-flash-error" role="alert">
            <strong>Could not load the service overview.</strong>{" "}
            {String(results.find((result) => result.error)?.error)}
          </div>
        </AwsContainer>
      </>
    );
  }

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
              <AwsStatus status="Operating normally" kind="success" />
            </div>
          </div>
        </AwsContainer>
        {SERVICES.map((service, index) => (
          <AwsContainer key={service.label}>
            <div className="aws-metric">
              <div className="aws-metric-label">{service.label}</div>
              <div className="aws-metric-value">
                <Link to={service.to}>{results[index].data?.length ?? 0}</Link>
              </div>
            </div>
          </AwsContainer>
        ))}
      </div>
    </>
  );
}
