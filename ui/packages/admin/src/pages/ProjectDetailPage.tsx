import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

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

  const {
    data: project,
    isLoading,
    isError,
    error,
  } = useQuery({
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
      navigate("/ui/topology");
    },
  });

  if (isLoading) return <Spinner label="loading project" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!project) return <Spinner label="loading project" />;

  const handleDelete = () => {
    if (
      window.confirm(`Delete project "${name}"? This will stop all components.`)
    ) {
      remove.mutate();
    }
  };

  const components = [
    { label: "Simulator", status: project.sim_status, port: project.sim_port },
    {
      label: "Backend",
      status: project.backend_status,
      port: project.backend_port,
    },
    {
      label: "Frontend",
      status: project.frontend_status,
      port: project.frontend_port,
    },
  ];

  const running = project.status === "running" || project.status === "partial";

  return (
    <div>
      <PageHeading
        kicker={`admin · ${cloudLabels[project.cloud] || project.cloud} project`}
        title={project.name}
        meta={
          <span className="inline-flex items-center gap-3">
            <StatusBadge status={statusLabel(project.status)} />
            <span>{project.backend}</span>
            {project.created_at && (
              <span>created {new Date(project.created_at).toLocaleDateString()}</span>
            )}
          </span>
        }
        actions={
          <span className="inline-flex gap-2">
            {running ? (
              <Button
                variant="danger"
                size="sm"
                onClick={() => stop.mutate()}
                disabled={stop.isPending}
              >
                {stop.isPending ? "Stopping…" : "Stop ⏹"}
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                onClick={() => start.mutate()}
                disabled={
                  start.isPending ||
                  project.status === "starting" ||
                  project.status === "stopping"
                }
              >
                {start.isPending ? "Starting…" : "Start ▶"}
              </Button>
            )}
            <Link
              to={`/ui/projects/${encodeURIComponent(name!)}/logs`}
              style={{ textDecoration: "none" }}
            >
              <Button variant="secondary" size="sm">
                View logs
              </Button>
            </Link>
            <Button
              variant="danger"
              size="sm"
              onClick={handleDelete}
              disabled={
                remove.isPending ||
                project.status === "starting" ||
                project.status === "stopping"
              }
            >
              {remove.isPending ? "Deleting…" : "Delete"}
            </Button>
          </span>
        }
      />

      {start.error && (
        <div className="mb-3">
          <ErrorPanel kicker="start failed" message={(start.error as Error)?.message} />
        </div>
      )}
      {stop.error && (
        <div className="mb-3">
          <ErrorPanel kicker="stop failed" message={(stop.error as Error)?.message} />
        </div>
      )}
      {remove.error && (
        <div className="mb-3">
          <ErrorPanel kicker="delete failed" message={(remove.error as Error)?.message} />
        </div>
      )}

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MetricsCard
          title="Cloud"
          value={cloudLabels[project.cloud] || project.cloud}
        />
        <MetricsCard title="Backend" value={project.backend} />
        <MetricsCard title="Log level" value={project.log_level || "default"} />
        <MetricsCard
          title="Created"
          value={
            project.created_at
              ? new Date(project.created_at).toLocaleDateString()
              : "—"
          }
        />
      </div>

      <h3
        className="mb-3 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        Components
      </h3>
      <div className="mb-6 grid gap-3 sm:grid-cols-3">
        {components.map((comp) => (
          <div
            key={comp.label}
            className="px-4 py-4"
            style={{
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderLeft:
                comp.status === "running"
                  ? "3px solid var(--color-status-ok)"
                  : "3px solid var(--color-border)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <div className="flex items-center justify-between gap-2 mb-2">
              <span
                className="font-mono uppercase tracking-[0.12em]"
                style={{ fontSize: "0.7rem", color: "var(--color-fg)" }}
              >
                {comp.label}
              </span>
              <StatusBadge status={statusLabel(comp.status || "stopped")} />
            </div>
            <div
              className="font-mono text-[12px]"
              style={{ color: "var(--color-fg-muted)" }}
            >
              port :{comp.port}
            </div>
          </div>
        ))}
      </div>

      {connection && (
        <section
          className="px-4 py-4"
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderLeft: "3px solid var(--color-accent)",
            borderRadius: "var(--radius-sm)",
          }}
        >
          <h3
            className="mb-3 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Connection
          </h3>
          <div className="space-y-2">
            <ConnectionField label="docker_host" value={connection.docker_host} />
            <ConnectionField label="env_export" value={connection.env_export} />
            <ConnectionField
              label="podman_connection"
              value={connection.podman_connection}
            />
          </div>
          <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4 font-mono text-[12px]">
            <KV k="simulator" v={connection.simulator_addr} />
            <KV k="backend" v={connection.backend_addr} />
            <KV k="frontend" v={connection.frontend_addr} />
            <KV k="mgmt" v={connection.frontend_mgmt_addr} />
          </div>
        </section>
      )}
    </div>
  );
}

function KV({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <div
        className="text-[10px] uppercase tracking-[0.18em] mb-0.5"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {k}
      </div>
      <div style={{ color: "var(--color-fg)" }}>{v}</div>
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
      // Clipboard API may fail in insecure contexts; quietly no-op.
    }
  };

  return (
    <div className="flex items-center gap-3">
      <span
        className="w-32 shrink-0 text-[10px] uppercase tracking-[0.18em] font-mono"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {label}
      </span>
      <code
        className="flex-1 px-2 py-1 font-mono"
        style={{
          background: "var(--color-bg-subtle)",
          border: "1px solid var(--color-border)",
          color: "var(--color-fg)",
          fontSize: "0.78rem",
          overflow: "auto",
        }}
      >
        {value}
      </code>
      <Button variant="ghost" size="sm" onClick={copy}>
        {copied ? "✓ copied" : "copy"}
      </Button>
    </div>
  );
}
