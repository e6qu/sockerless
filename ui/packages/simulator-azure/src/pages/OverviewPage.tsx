import { Link } from "react-router";
import { useQueries } from "@tanstack/react-query";
import { AzureEssentials, AzureStatus } from "../portal/index.js";
import { AzureCommandBar } from "../portal/AzurePortal.js";
import {
  fetchContainerAppJobs,
  fetchFunctionSites,
  fetchACRRegistries,
  fetchStorageAccounts,
  fetchMonitorLogs,
} from "../api.js";

// The overview counts the real resources by reading the same Azure APIs each
// service page reads, so the board reflects the subscription rather than a
// separate summary endpoint.
const RESOURCES = [
  { label: "Container Apps jobs", to: "/ui/container-apps", queryKey: ["ca-jobs"], queryFn: fetchContainerAppJobs },
  { label: "Function Apps", to: "/ui/functions", queryKey: ["fn-sites"], queryFn: fetchFunctionSites },
  { label: "Container registries", to: "/ui/acr", queryKey: ["acr-registries"], queryFn: fetchACRRegistries },
  { label: "Storage accounts", to: "/ui/storage", queryKey: ["storage-accounts"], queryFn: fetchStorageAccounts },
  { label: "Log entries", to: "/ui/monitor", queryKey: ["monitor-logs"], queryFn: fetchMonitorLogs },
] as const;

export function OverviewPage() {
  const results = useQueries({
    queries: RESOURCES.map((resource) => ({ queryKey: resource.queryKey, queryFn: resource.queryFn })),
  });

  // A failed read must not render an all-zeros board that reads as healthy.
  const failed = results.some((result) => result.isError);
  const loading = results.some((result) => result.isLoading);
  const firstError = results.find((result) => result.isError)?.error;

  return (
    <>
      <AzureCommandBar
        commands={[
          { label: "Refresh", icon: "refresh", onSelect: () => results.forEach((result) => void result.refetch()) },
          { label: "Feedback", icon: "feedback" },
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
                <AzureStatus status="Available" />
              ),
            },
            { label: "Resource types", value: String(RESOURCES.length) },
          ]}
        />
        {failed ? (
          <div className="az-message az-message-error" role="alert" data-testid="overview-error">
            <strong>Could not load the subscription overview.</strong>{" "}
            {firstError instanceof Error ? firstError.message : "The portal could not reach Azure."}
          </div>
        ) : (
          <div className="az-cards">
            {RESOURCES.map((resource, index) => (
              <div className="az-card" key={resource.label}>
                <h2>{resource.label}</h2>
                <Link to={resource.to}>{loading ? "—" : (results[index].data?.length ?? 0)}</Link>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
