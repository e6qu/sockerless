import { useQuery } from "@tanstack/react-query";
import { Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient, type AdminComponent } from "../api.js";

const api = new AdminApiClient();

function ComponentMetricsPanel({ component }: { component: AdminComponent }) {
  const { data, isError, error } = useQuery({
    queryKey: ["component-metrics", component.name],
    queryFn: () => api.componentMetrics(component.name),
    enabled: component.health === "up",
  });

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h3 className="mb-2 font-medium">{component.name}</h3>
      <p className="text-xs text-gray-500 dark:text-gray-400">{component.type}</p>
      {isError ? (
        <p className="mt-2 text-sm text-red-600 dark:text-red-400">
          Error: {error?.message ?? "Failed to load metrics"}
        </p>
      ) : data ? (
        <pre className="mt-2 max-h-64 overflow-auto text-xs">
          {JSON.stringify(data, null, 2)}
        </pre>
      ) : (
        <p className="mt-2 text-sm text-gray-400">
          {component.health === "up" ? "Loading..." : "Unavailable"}
        </p>
      )}
    </div>
  );
}

export function MetricsPage() {
  const { data: components, isLoading, isError, error } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
  });

  if (isLoading) return <Spinner />;
  if (isError) return <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">Error: {error?.message ?? "Failed to load"}</div>;
  if (!components) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Metrics</h2>
      {components.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">No components found.</p>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {components.map((c) => (
            <ComponentMetricsPanel key={c.name} component={c} />
          ))}
        </div>
      )}
    </div>
  );
}
