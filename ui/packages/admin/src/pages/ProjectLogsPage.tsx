import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { LogViewer, Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";

const api = new AdminApiClient();

const components = [
  { value: "", label: "All" },
  { value: "sim", label: "Simulator" },
  { value: "backend", label: "Backend" },
  { value: "frontend", label: "Frontend" },
];

export function ProjectLogsPage() {
  const { name } = useParams<{ name: string }>();
  const [component, setComponent] = useState("");

  const { data: logs, isLoading, isError, error } = useQuery({
    queryKey: ["project-logs", name, component],
    queryFn: () => api.projectLogs(name!, component || undefined, 200),
    enabled: !!name,
    refetchInterval: 2000,
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link
          to={`/ui/projects/${encodeURIComponent(name!)}`}
          className="text-sm text-blue-600 hover:underline dark:text-blue-400"
        >
          Back to {name}
        </Link>
        <h2 className="text-xl font-semibold">Logs</h2>
      </div>

      {/* Component selector */}
      <div className="flex gap-1">
        {components.map((c) => (
          <button
            key={c.value}
            onClick={() => setComponent(c.value)}
            className={`rounded-md px-3 py-1.5 text-sm font-medium ${
              component === c.value
                ? "bg-blue-600 text-white"
                : "bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600"
            }`}
          >
            {c.label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner />
      ) : isError ? (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          Error: {error?.message ?? "Failed to load logs"}
        </div>
      ) : (
        <LogViewer lines={logs ?? []} />
      )}
    </div>
  );
}
