import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { StatusBadge, MetricsCard, Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";

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
    default:
      return status;
  }
}

export function ProjectDetailPage() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: project, isLoading, isError, error } = useQuery({
    queryKey: ["project", name],
    queryFn: () => api.projectGet(name!),
    enabled: !!name,
    refetchInterval: 3000,
  });

  const { data: connection } = useQuery({
    queryKey: ["project-connection", name],
    queryFn: () => api.projectConnection(name!),
    enabled: !!name,
  });

  const start = useMutation({
    mutationFn: () => api.projectStart(name!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project", name] });
      queryClient.invalidateQueries({ queryKey: ["project-connection", name] });
    },
  });

  const stop = useMutation({
    mutationFn: () => api.projectStop(name!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project", name] });
      queryClient.invalidateQueries({ queryKey: ["project-connection", name] });
    },
  });

  const remove = useMutation({
    mutationFn: () => api.projectDelete(name!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate("/ui/projects");
    },
  });

  if (isLoading) return <Spinner />;
  if (isError) return <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">Error: {error?.message ?? "Failed to load project"}</div>;
  if (!project) return <Spinner />;

  const handleDelete = () => {
    if (window.confirm(`Delete project "${name}"? This will stop all components.`)) {
      remove.mutate();
    }
  };

  const components = [
    { label: "Simulator", status: project.sim_status, port: project.sim_port },
    { label: "Backend", status: project.backend_status, port: project.backend_port },
    { label: "Frontend", status: project.frontend_status, port: project.frontend_port },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <h2 className="text-xl font-semibold">{project.name}</h2>
        <StatusBadge status={statusLabel(project.status)} />
      </div>

      {/* Info grid */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <MetricsCard title="Cloud" value={cloudLabels[project.cloud] || project.cloud} />
        <MetricsCard title="Backend" value={project.backend} />
        <MetricsCard title="Log Level" value={project.log_level || "default"} />
        <MetricsCard
          title="Created"
          value={project.created_at ? new Date(project.created_at).toLocaleDateString() : "-"}
        />
      </div>

      {/* Actions */}
      <div className="flex gap-2">
        {project.status === "running" || project.status === "partial" ? (
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
            disabled={start.isPending || project.status === "starting" || project.status === "stopping"}
            className="rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
          >
            {start.isPending ? "Starting..." : "Start"}
          </button>
        )}
        <Link
          to={`/ui/projects/${encodeURIComponent(name!)}/logs`}
          className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
        >
          View Logs
        </Link>
        <button
          onClick={handleDelete}
          disabled={remove.isPending || project.status === "starting" || project.status === "stopping"}
          className="rounded-md border border-red-300 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-700 dark:text-red-400 dark:hover:bg-red-900/30"
        >
          {remove.isPending ? "Deleting..." : "Delete"}
        </button>
      </div>

      {[start.error, stop.error, remove.error].filter(Boolean).map((e, i) => (
        <div key={i} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          {(e as Error)?.message}
        </div>
      ))}

      {/* Components */}
      <div>
        <h3 className="mb-3 text-lg font-medium">Components</h3>
        <div className="grid gap-4 sm:grid-cols-3">
          {components.map((comp) => (
            <div
              key={comp.label}
              className="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
            >
              <div className="mb-2 flex items-center justify-between">
                <span className="font-medium">{comp.label}</span>
                <StatusBadge status={statusLabel(comp.status || "stopped")} />
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Port: {comp.port}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Connection Info */}
      {connection && (
        <div>
          <h3 className="mb-3 text-lg font-medium">Connection Info</h3>
          <div className="space-y-3">
            <ConnectionField label="Docker Host" value={connection.docker_host} />
            <ConnectionField label="Export Command" value={connection.env_export} />
            <ConnectionField label="Podman Connection" value={connection.podman_connection} />
          </div>
          <div className="mt-4 grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <div>
              <span className="font-medium text-gray-700 dark:text-gray-300">Simulator:</span>{" "}
              <span className="text-gray-500 dark:text-gray-400">{connection.simulator_addr}</span>
            </div>
            <div>
              <span className="font-medium text-gray-700 dark:text-gray-300">Backend:</span>{" "}
              <span className="text-gray-500 dark:text-gray-400">{connection.backend_addr}</span>
            </div>
            <div>
              <span className="font-medium text-gray-700 dark:text-gray-300">Frontend:</span>{" "}
              <span className="text-gray-500 dark:text-gray-400">{connection.frontend_addr}</span>
            </div>
            <div>
              <span className="font-medium text-gray-700 dark:text-gray-300">Mgmt:</span>{" "}
              <span className="text-gray-500 dark:text-gray-400">{connection.frontend_mgmt_addr}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function ConnectionField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API may fail in insecure contexts
    }
  };

  return (
    <div className="flex items-center gap-2">
      <span className="w-40 shrink-0 text-sm font-medium text-gray-700 dark:text-gray-300">{label}:</span>
      <code className="flex-1 rounded bg-gray-100 px-2 py-1 text-sm dark:bg-gray-700">{value}</code>
      <button
        onClick={copy}
        className="shrink-0 rounded border border-gray-300 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
      >
        {copied ? "Copied!" : "Copy"}
      </button>
    </div>
  );
}
