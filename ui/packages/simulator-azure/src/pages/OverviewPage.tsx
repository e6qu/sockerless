import { Link } from "react-router";
import { useSimSummary, useSimHealth } from "@sockerless/ui-core/hooks";
import { AzureEssentials, AzureStatus } from "../portal/index.js";
import { AzureCommandBar } from "../portal/AzurePortal.js";

const RESOURCES = [
  { label: "Container Apps jobs", key: "container_app_jobs", to: "/ui/container-apps" },
  { label: "Function Apps", key: "function_sites", to: "/ui/functions" },
  { label: "Container registries", key: "acr_registries", to: "/ui/acr" },
  { label: "Storage accounts", key: "storage_accounts", to: "/ui/storage" },
  { label: "Log entries", key: "monitor_logs", to: "/ui/monitor" },
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
      <AzureCommandBar
        commands={[
          { label: "Refresh", glyph: "↻", onSelect: () => { void health.refetch(); void summary.refetch(); } },
          { label: "Feedback", glyph: "☺" },
        ]}
      />
      <div className="az-main">
        <AzureEssentials
          properties={[
            { label: "Subscription", value: "Simulator" },
            { label: "Directory", value: "Simulator" },
            {
              label: "Status",
              value: failed ? (
                <AzureStatus status="Unavailable" kind="error" />
              ) : loading ? (
                <AzureStatus status="Loading" kind="warning" />
              ) : (
                <AzureStatus status={health.data?.status === "ok" ? "Available" : "Unavailable"} />
              ),
            },
            { label: "Resource types", value: String(RESOURCES.length) },
          ]}
        />
        {failed ? (
          <div className="az-message az-message-error" role="alert">
            <strong>Could not load the subscription overview.</strong>{" "}
            {String(summary.error ?? health.error)}
          </div>
        ) : (
          <div className="az-cards">
            {RESOURCES.map((resource) => (
              <div className="az-card" key={resource.key}>
                <h2>{resource.label}</h2>
                <Link to={resource.to}>{loading ? "—" : (services[resource.key] ?? 0)}</Link>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
