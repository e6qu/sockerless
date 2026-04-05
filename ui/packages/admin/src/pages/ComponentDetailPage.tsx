import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  StatusBadge,
  MetricsCard,
  Spinner,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type AdminComponent,
  type ComponentStatus,
  type ComponentMetrics,
} from "../api.js";

const api = new AdminApiClient();

export function ComponentDetailPage() {
  const { name } = useParams<{ name: string }>();
  const queryClient = useQueryClient();

  const {
    data: components,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
  });

  const comp = components?.find((c: AdminComponent) => c.name === name);

  const { data: status } = useQuery({
    queryKey: ["component-status", name],
    queryFn: () => api.componentStatus(name!),
    enabled: !!name,
    refetchInterval: 5000,
  });

  const { data: metrics } = useQuery({
    queryKey: ["component-metrics", name],
    queryFn: () => api.componentMetrics(name!),
    enabled: !!name,
    refetchInterval: 5000,
  });

  const { data: provider } = useQuery({
    queryKey: ["component-provider", name],
    queryFn: () => api.componentProvider(name!),
    enabled: !!name && comp?.type === "backend",
  });

  const reload = useMutation({
    mutationFn: () => api.componentReload(name!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["components"] });
      queryClient.invalidateQueries({ queryKey: ["component-status", name] });
      queryClient.invalidateQueries({ queryKey: ["component-metrics", name] });
      queryClient.invalidateQueries({ queryKey: ["component-provider", name] });
    },
  });

  if (isLoading) return <Spinner />;
  if (isError)
    return (
      <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
        Error: {error?.message ?? "Failed to load"}
      </div>
    );
  if (!components) return <Spinner />;
  if (!comp)
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400">
        Component &quot;{name}&quot; not found
      </div>
    );

  const statusObj: ComponentStatus | undefined = status;
  const metricsObj: ComponentMetrics | undefined = metrics;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h2 className="text-xl font-semibold">{comp.name}</h2>
        <StatusBadge
          status={
            comp.health === "up"
              ? "ok"
              : comp.health === "unknown"
                ? "warning"
                : "error"
          }
        />
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Type" value={comp.type} />
        <MetricsCard title="Address" value={comp.addr} />
        <MetricsCard
          title="Uptime"
          value={
            comp.uptime > 0
              ? (() => {
                  const h = Math.floor(comp.uptime / 3600);
                  const m = Math.floor((comp.uptime % 3600) / 60);
                  return h > 0 ? `${h}h ${m}m` : `${m}m`;
                })()
              : "-"
          }
        />
        <MetricsCard
          title="Containers"
          value={
            statusObj?.containers != null ? String(statusObj.containers) : "-"
          }
        />
      </div>

      {(comp.type === "backend" || comp.type === "frontend") && (
        <button
          onClick={() => reload.mutate()}
          disabled={reload.isPending}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {reload.isPending ? "Reloading..." : "Reload"}
        </button>
      )}

      {reload.error && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          {(reload.error as Error)?.message}
        </div>
      )}

      {provider && comp.type === "backend" && (
        <div className="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          <h3 className="mb-3 text-lg font-medium">Cloud Connection</h3>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                Provider
              </p>
              <p className="font-medium">{provider.provider}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500 dark:text-gray-400">Mode</p>
              <span
                className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                  provider.mode === "cloud"
                    ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
                    : provider.mode === "custom-endpoint"
                      ? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
                      : "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300"
                }`}
              >
                {provider.mode}
              </span>
            </div>
            {provider.region && (
              <div>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  Region
                </p>
                <p className="font-medium">{provider.region}</p>
              </div>
            )}
          </div>
          {provider.endpoint && (
            <div className="mt-3">
              <p className="text-xs text-gray-500 dark:text-gray-400">
                Endpoint
              </p>
              <p className="text-sm font-mono">{provider.endpoint}</p>
            </div>
          )}
          {provider.resources && Object.keys(provider.resources).length > 0 && (
            <div className="mt-3">
              <p className="mb-1 text-xs text-gray-500 dark:text-gray-400">
                Resources
              </p>
              <div className="space-y-1">
                {Object.entries(provider.resources).map(([key, value]) => (
                  <div key={key} className="flex gap-2 text-sm">
                    <span className="text-gray-500 dark:text-gray-400">
                      {key}:
                    </span>
                    <span className="font-mono">{value}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {statusObj && (
        <div>
          <h3 className="mb-2 text-lg font-medium">Status</h3>
          <pre className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm dark:border-gray-700 dark:bg-gray-800">
            {JSON.stringify(statusObj, null, 2)}
          </pre>
        </div>
      )}

      {metricsObj && (
        <div>
          <h3 className="mb-2 text-lg font-medium">Metrics</h3>
          <pre className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm dark:border-gray-700 dark:bg-gray-800">
            {JSON.stringify(metricsObj, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}
