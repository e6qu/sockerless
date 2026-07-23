import { Link } from "react-router";
import { useSimSummary, useSimHealth } from "@sockerless/ui-core/hooks";
import { GcpPageHeader, GcpStatus } from "../console/GcpConsole.js";

const RESOURCES = [
  { label: "Cloud Run jobs", key: "cloudrun_jobs", to: "/ui/cloudrun" },
  { label: "Cloud Run functions", key: "functions", to: "/ui/functions" },
  { label: "Artifact Registry repositories", key: "ar_repos", to: "/ui/ar" },
  { label: "Cloud Storage buckets", key: "gcs_buckets", to: "/ui/gcs" },
  { label: "Log entries", key: "log_entries", to: "/ui/logging" },
] as const;

export function OverviewPage() {
  const health = useSimHealth();
  const summary = useSimSummary();
  const services: Record<string, number> = summary.data?.services ?? {};

  // A failed fetch must not render an all-zeros board that reads as healthy.
  const failed = health.isError || summary.isError;
  const loading = health.isLoading || summary.isLoading;

  return (
    <>
      <GcpPageHeader
        title="Overview"
        description="Resources across this project."
        onRefresh={() => { void health.refetch(); void summary.refetch(); }}
        refreshing={health.isFetching || summary.isFetching}
      />
      <div style={{ marginBottom: 16 }}>
        {failed ? (
          <GcpStatus status="Unavailable" kind="error" />
        ) : loading ? (
          <GcpStatus status="Loading" kind="warning" />
        ) : (
          <GcpStatus status={health.data?.status === "ok" ? "All services operating normally" : "Unavailable"} />
        )}
      </div>
      {failed ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't load the project overview.</strong>{" "}
          {String(summary.error ?? health.error)}
        </div>
      ) : (
        <div className="gc-cards">
          {RESOURCES.map((resource) => (
            <div className="gc-card" key={resource.key}>
              <h2>{resource.label}</h2>
              <Link to={resource.to}>{loading ? "—" : (services[resource.key] ?? 0)}</Link>
            </div>
          ))}
        </div>
      )}
    </>
  );
}
