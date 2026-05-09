import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
  StatusBadge,
  useToast,
  useReportError,
} from "@sockerless/ui-core/components";
import { Link, useNavigate } from "react-router";
import { AdminApiClient, type ProjectStatus } from "../api.js";
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
    case "starting":
      return "starting";
    default:
      return status;
  }
}

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { push } = useToast();
  const reportError = useReportError();

  const {
    data: projects,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["projects"],
    queryFn: () => api.projects(),
    refetchInterval: 3000,
  });

  const start = useMutation({
    mutationFn: (name: string) => api.projectStart(name),
    onSuccess: (_data, name) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      push({ tone: "success", title: `Started ${name}` });
    },
    onError: (err, name) => reportError(err, `Failed to start ${name}`),
  });

  const stop = useMutation({
    mutationFn: (name: string) => api.projectStop(name),
    onSuccess: (_data, name) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      push({ tone: "success", title: `Stopped ${name}` });
    },
    onError: (err, name) => reportError(err, `Failed to stop ${name}`),
  });

  if (isLoading) return <Spinner label="loading projects" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!projects) return <Spinner label="loading projects" />;

  return (
    <div>
      <PageHeading
        kicker="admin · projects"
        title={<>Projects</>}
        meta={`${projects.length} project${projects.length === 1 ? "" : "s"} configured`}
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => navigate("/ui/projects/new")}
          >
            + new project
          </Button>
        }
      />

      {projects.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no projects configured —
        </p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((proj: ProjectStatus, i) => (
            <div
              key={proj.name}
              className="reveal flex flex-col"
              style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderLeft:
                  proj.status === "running"
                    ? "3px solid var(--color-status-ok)"
                    : proj.status === "failed"
                      ? "3px solid var(--color-status-error)"
                      : "3px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                "--reveal-delay": `${i * 30}ms`,
              } as React.CSSProperties}
            >
              <div
                className="px-4 py-3"
                style={{ borderBottom: "1px solid var(--color-border)" }}
              >
                <div className="flex items-center justify-between gap-2">
                  <Link
                    to={`/ui/projects/${encodeURIComponent(proj.name)}`}
                    className="font-display"
                    style={{
                      fontStyle: "italic",
                      fontWeight: 600,
                      fontSize: "1.2rem",
                      letterSpacing: "-0.02em",
                      color: "var(--color-fg)",
                      textDecoration: "none",
                    }}
                  >
                    {proj.name}
                  </Link>
                  <StatusBadge status={statusLabel(proj.status)} />
                </div>
                <div
                  className="mt-1 text-[10px] uppercase tracking-[0.2em]"
                  style={{ color: "var(--color-fg-subtle)" }}
                >
                  {cloudLabels[proj.cloud] || proj.cloud} · {proj.backend}
                </div>
              </div>

              <div
                className="px-4 py-3 font-mono text-[12px] flex-1"
                style={{ color: "var(--color-fg-muted)" }}
              >
                <div>
                  <span style={{ color: "var(--color-fg-subtle)" }}>
                    frontend
                  </span>
                  <span className="ml-2" style={{ color: "var(--color-fg)" }}>
                    :{proj.frontend_port}
                  </span>
                </div>
                <div>
                  <span style={{ color: "var(--color-fg-subtle)" }}>
                    backend
                  </span>
                  <span className="ml-2" style={{ color: "var(--color-fg)" }}>
                    :{proj.backend_port}
                  </span>
                </div>
                <div>
                  <span style={{ color: "var(--color-fg-subtle)" }}>sim</span>
                  <span className="ml-2" style={{ color: "var(--color-fg)" }}>
                    :{proj.sim_port}
                  </span>
                </div>
              </div>

              <div
                className="px-4 py-3 flex justify-end"
                style={{ borderTop: "1px solid var(--color-border)" }}
              >
                {proj.status === "running" || proj.status === "partial" ? (
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={(e) => {
                      e.preventDefault();
                      stop.mutate(proj.name);
                    }}
                    disabled={stop.isPending && stop.variables === proj.name}
                  >
                    Stop ⏹
                  </Button>
                ) : (
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={(e) => {
                      e.preventDefault();
                      start.mutate(proj.name);
                    }}
                    disabled={
                      (start.isPending && start.variables === proj.name) ||
                      proj.status === "starting" ||
                      proj.status === "stopping"
                    }
                  >
                    Start ▶
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
