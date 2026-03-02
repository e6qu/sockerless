import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { Link } from "react-router";
import { AdminApiClient, type ProjectStatus } from "../api.js";

const api = new AdminApiClient();

const cloudLabels: Record<string, string> = {
  aws: "AWS",
  gcp: "GCP",
  azure: "Azure",
};

function statusLabel(status: string): string {
  switch (status) {
    case "running":
      return "running";
    case "failed":
      return "error";
    case "partial":
      return "warning";
    case "starting":
      return "starting";
    default:
      return status;
  }
}

export function ProjectsPage() {
  const queryClient = useQueryClient();

  const { data: projects, isLoading } = useQuery({
    queryKey: ["projects"],
    queryFn: () => api.projects(),
    refetchInterval: 3000,
  });

  const start = useMutation({
    mutationFn: (name: string) => api.projectStart(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });

  const stop = useMutation({
    mutationFn: (name: string) => api.projectStop(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["projects"] }),
  });

  if (isLoading || !projects) return <Spinner />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Projects</h2>
        <Link
          to="/ui/projects/new"
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          New Project
        </Link>
      </div>

      {projects.length === 0 ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          No projects configured. Create a project to get started with a simulator + backend + frontend environment.
        </p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((proj: ProjectStatus) => (
            <div
              key={proj.name}
              className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800"
            >
              <div className="mb-3 flex items-center justify-between">
                <Link
                  to={`/ui/projects/${proj.name}`}
                  className="text-lg font-medium text-blue-600 hover:underline dark:text-blue-400"
                >
                  {proj.name}
                </Link>
                <StatusBadge status={statusLabel(proj.status)} />
              </div>

              <div className="mb-3 space-y-1 text-sm text-gray-500 dark:text-gray-400">
                <div>
                  <span className="font-medium text-gray-700 dark:text-gray-300">Cloud:</span>{" "}
                  {cloudLabels[proj.cloud] || proj.cloud}
                </div>
                <div>
                  <span className="font-medium text-gray-700 dark:text-gray-300">Backend:</span>{" "}
                  {proj.backend}
                </div>
                <div>
                  <span className="font-medium text-gray-700 dark:text-gray-300">Ports:</span>{" "}
                  {proj.frontend_port} / {proj.backend_port} / {proj.sim_port}
                </div>
              </div>

              <div className="flex gap-2">
                {proj.status === "running" || proj.status === "partial" ? (
                  <button
                    onClick={(e) => { e.preventDefault(); stop.mutate(proj.name); }}
                    disabled={stop.isPending}
                    className="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    Stop
                  </button>
                ) : (
                  <button
                    onClick={(e) => { e.preventDefault(); start.mutate(proj.name); }}
                    disabled={start.isPending || proj.status === "starting"}
                    className="rounded-md bg-green-600 px-3 py-1 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
                  >
                    Start
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
