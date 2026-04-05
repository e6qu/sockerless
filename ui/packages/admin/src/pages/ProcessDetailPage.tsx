import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  StatusBadge,
  MetricsCard,
  Spinner,
  LogViewer,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";

const api = new AdminApiClient();

export function ProcessDetailPage() {
  const { name } = useParams<{ name: string }>();
  const queryClient = useQueryClient();

  const {
    data: processes,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["processes"],
    queryFn: () => api.processes(),
    refetchInterval: 3000,
  });

  const { data: logs } = useQuery({
    queryKey: ["process-logs", name],
    queryFn: () => api.processLogs(name!, 200),
    enabled: !!name,
    refetchInterval: 2000,
  });

  const start = useMutation({
    mutationFn: () => api.processStart(name!),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["processes"] }),
  });

  const stop = useMutation({
    mutationFn: () => api.processStop(name!),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["processes"] }),
  });

  const proc = processes?.find((p) => p.name === name);

  if (isLoading) return <Spinner />;
  if (isError)
    return (
      <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
        Error: {error?.message ?? "Failed to load"}
      </div>
    );
  if (!processes) return <Spinner />;
  if (!proc)
    return (
      <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400">
        Process &quot;{name}&quot; not found
      </div>
    );

  const statusStr =
    proc.status === "running"
      ? "running"
      : proc.status === "failed"
        ? "error"
        : proc.status;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h2 className="text-xl font-semibold">{proc.name}</h2>
        <StatusBadge status={statusStr} />
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Binary" value={proc.binary} />
        <MetricsCard title="PID" value={proc.pid || "-"} />
        <MetricsCard title="Address" value={proc.addr || "-"} />
        <MetricsCard
          title="Started"
          value={
            proc.started_at
              ? new Date(proc.started_at).toLocaleTimeString()
              : "-"
          }
        />
      </div>

      <div className="flex gap-2">
        {proc.status === "running" ? (
          <button
            onClick={() => stop.mutate()}
            disabled={stop.isPending}
            className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
          >
            {stop.isPending ? "Stopping..." : "Stop"}
          </button>
        ) : (
          <button
            onClick={() => start.mutate()}
            disabled={
              start.isPending ||
              proc.status === "starting" ||
              proc.status === "stopping"
            }
            className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
          >
            {start.isPending ? "Starting..." : "Start"}
          </button>
        )}
      </div>

      {[start.error, stop.error].filter(Boolean).map((e, i) => (
        <div
          key={i}
          className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400"
        >
          {(e as Error)?.message}
        </div>
      ))}

      {proc.exit_code !== 0 && proc.status !== "running" && (
        <p className="text-sm text-red-600 dark:text-red-400">
          Exit code: {proc.exit_code}
        </p>
      )}

      <div>
        <h3 className="mb-2 text-lg font-medium">Logs</h3>
        <LogViewer lines={logs ?? []} />
      </div>
    </div>
  );
}
