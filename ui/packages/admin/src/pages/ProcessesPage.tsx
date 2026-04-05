import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { Link } from "react-router";
import { AdminApiClient, type ProcessInfo } from "../api.js";

const api = new AdminApiClient();

function statusLabel(status: string): string {
  // Map process status to StatusBadge-compatible status string
  switch (status) {
    case "running":
      return "running";
    case "failed":
      return "error";
    default:
      return status;
  }
}

export function ProcessesPage() {
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

  const start = useMutation({
    mutationFn: (name: string) => api.processStart(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["processes"] }),
  });

  const stop = useMutation({
    mutationFn: (name: string) => api.processStop(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["processes"] }),
  });

  if (isLoading) return <Spinner />;
  if (isError)
    return (
      <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
        Error: {error?.message ?? "Failed to load"}
      </div>
    );
  if (!processes) return <Spinner />;

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">Processes</h2>

      {processes.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          No managed processes configured. Add process definitions to
          admin.json.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Name
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Binary
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Status
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  PID
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Address
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Uptime
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {processes.map((proc: ProcessInfo) => (
                <tr key={proc.name}>
                  <td className="whitespace-nowrap px-4 py-3 text-sm font-medium">
                    <Link
                      to={`/ui/processes/${encodeURIComponent(proc.name)}`}
                      className="text-blue-600 hover:underline dark:text-blue-400"
                    >
                      {proc.name}
                    </Link>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {proc.binary}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm">
                    <StatusBadge status={statusLabel(proc.status)} />
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {proc.pid || "-"}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {proc.addr || "-"}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                    {proc.started_at ? formatUptime(proc.started_at) : "-"}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-sm">
                    {proc.status === "running" ? (
                      <button
                        onClick={() => stop.mutate(proc.name)}
                        disabled={
                          stop.isPending && stop.variables === proc.name
                        }
                        className="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                      >
                        Stop
                      </button>
                    ) : (
                      <button
                        onClick={() => start.mutate(proc.name)}
                        disabled={
                          (start.isPending && start.variables === proc.name) ||
                          proc.status === "starting" ||
                          proc.status === "stopping"
                        }
                        className="rounded-md bg-green-600 px-3 py-1 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
                      >
                        Start
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {[start.error, stop.error].filter(Boolean).map((e, i) => (
        <div
          key={i}
          className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400"
        >
          {(e as Error)?.message}
        </div>
      ))}
    </div>
  );
}

function formatUptime(startedAt: string): string {
  const start = new Date(startedAt);
  const seconds = Math.floor((Date.now() - start.getTime()) / 1000);
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}
