import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { StatusBadge, MetricsCard, Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient, type AdminComponent } from "../api.js";

const api = new AdminApiClient();

export function ComponentDetailPage() {
  const { name } = useParams<{ name: string }>();
  const queryClient = useQueryClient();

  const { data: components } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
  });

  const { data: status } = useQuery({
    queryKey: ["component-status", name],
    queryFn: () => api.componentStatus(name!),
    enabled: !!name,
  });

  const { data: metrics } = useQuery({
    queryKey: ["component-metrics", name],
    queryFn: () => api.componentMetrics(name!),
    enabled: !!name,
  });

  const reload = useMutation({
    mutationFn: () => api.componentReload(name!),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["components"] }),
  });

  const comp = components?.find((c: AdminComponent) => c.name === name);

  if (!comp) return <Spinner />;

  const statusObj = status as Record<string, unknown> | undefined;
  const metricsObj = metrics as Record<string, unknown> | undefined;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h2 className="text-xl font-semibold">{comp.name}</h2>
        <StatusBadge status={comp.health === "up" ? "ok" : "error"} />
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Type" value={comp.type} />
        <MetricsCard title="Address" value={comp.addr} />
        <MetricsCard
          title="Uptime"
          value={comp.uptime > 0 ? `${Math.floor(comp.uptime / 60)}m` : "-"}
        />
        <MetricsCard
          title="Containers"
          value={statusObj?.containers != null ? String(statusObj.containers) : "-"}
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
