import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { LogViewer, Spinner } from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  buildObservabilityURL,
  type InstanceStatus,
} from "../api.js";

const api = new AdminApiClient();

export interface UnhealthyDiagnosticPanelProps {
  project: string;
  instanceName: string;
  /** The latest InstanceStatus from the row's polling query. */
  status: InstanceStatus;
}

/**
 * UnhealthyDiagnosticPanel — collapsible panel rendered under an
 * InstanceRow when the row's status indicates a problem (health =
 * unhealthy, or crashed_since_start, or process gone unexpectedly).
 *
 * Shows: failing signal, exit info if present, last N lines of
 * `.stack-pids/<name>.log` via the diagnostic endpoint, deep links to
 * the live log tail and project console.
 *
 * Data source is `/api/v1/topology/.../diagnostics` — one request per
 * mount/refresh, polled every 10 s while open. The panel only mounts
 * when shouldRender(status) === true, so the polling cost is bounded
 * to actually-broken instances.
 */
export function UnhealthyDiagnosticPanel({
  project,
  instanceName,
  status,
}: UnhealthyDiagnosticPanelProps) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["instance-diagnostics", project, instanceName],
    queryFn: () => api.topologyInstanceDiagnostics(project, instanceName),
    refetchInterval: 10000,
    staleTime: 5000,
  });

  // Observability config is admin-boot static — fetch via react-query
  // so multiple panels share one cached entry. shareable cache key
  // matches TopologyPage if it ever wants to read the same value.
  const { data: obs } = useQuery({
    queryKey: ["observability"],
    queryFn: () => api.observability(),
    staleTime: 5 * 60_000,
  });

  const logsURL = obs?.enabled
    ? buildObservabilityURL(obs.logs_dashboard, obs.logs_service_param, instanceName)
    : "";
  const tracesURL = obs?.enabled
    ? buildObservabilityURL(obs.traces_dashboard, obs.traces_service_param, instanceName)
    : "";

  const reason = describeReason(status);

  return (
    <div
      className="px-4 pt-2 pb-3"
      style={{
        background: "var(--color-status-warn-soft)",
        borderTop: "1px solid var(--color-status-warn)",
      }}
    >
      <div className="flex flex-wrap items-center gap-3 mb-2">
        <span
          className="font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-status-warn)", fontSize: "0.62rem" }}
        >
          {reason}
        </span>
        {status.exit && (
          <span
            className="font-mono"
            style={{ color: "var(--color-fg-muted)", fontSize: "0.7rem" }}
          >
            exit {status.exit.code} @ {status.exit.at}
          </span>
        )}
        <span className="ml-auto inline-flex items-center gap-2">
          <Link
            to={`/ui/topology/${encodeURIComponent(project)}/${encodeURIComponent(instanceName)}/logs`}
            style={chipStyle}
          >
            full logs
          </Link>
          <Link
            to={`/ui/topology/${encodeURIComponent(project)}/console`}
            style={chipStyle}
          >
            console
          </Link>
          {logsURL && (
            <a
              href={logsURL}
              target="_blank"
              rel="noopener noreferrer"
              style={{ ...chipStyle, color: "var(--color-accent)" }}
            >
              VictoriaLogs ↗
            </a>
          )}
          {tracesURL && (
            <a
              href={tracesURL}
              target="_blank"
              rel="noopener noreferrer"
              style={{ ...chipStyle, color: "var(--color-accent)" }}
            >
              Jaeger ↗
            </a>
          )}
          <button
            type="button"
            onClick={() => {
              void refetch();
            }}
            className="font-mono uppercase tracking-[0.16em]"
            style={{
              fontSize: "0.6rem",
              padding: "0.2rem 0.5rem",
              background: "transparent",
              color: "var(--color-fg-muted)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-xs)",
              cursor: "pointer",
            }}
          >
            refresh
          </button>
        </span>
      </div>

      {isLoading && <Spinner label="loading diagnostics" />}
      {isError && (
        <p
          className="font-mono"
          style={{ color: "var(--color-status-error)", fontSize: "0.78rem" }}
        >
          {error instanceof Error ? error.message : "diagnostic fetch failed"}
        </p>
      )}
      {data && (
        <>
          {status.health_detail && (
            <p
              className="font-mono mb-2"
              style={{
                color: "var(--color-status-error)",
                fontSize: "0.78rem",
              }}
            >
              {status.health_detail}
            </p>
          )}
          <div
            className="font-mono uppercase tracking-[0.18em] mb-1"
            style={{ color: "var(--color-fg-subtle)", fontSize: "0.6rem" }}
          >
            last {data.log_lines.length} log line
            {data.log_lines.length === 1 ? "" : "s"}
          </div>
          <LogViewer lines={data.log_lines} maxHeight="20rem" />
        </>
      )}
    </div>
  );
}

/**
 * shouldRender returns true when the status indicates a problem the
 * operator should look at. Keeps the panel mounted only on actually-
 * broken rows so the diagnostic poll fires sparingly.
 */
export function shouldRender(status: InstanceStatus | undefined): boolean {
  if (!status) return false;
  if (status.health === "unhealthy") return true;
  if (status.crashed_since_start) return true;
  // Process gone but a PID was recorded → unexpected disappearance.
  if (!status.running && status.pid > 0) return true;
  return false;
}

const chipStyle = {
  fontSize: "0.65rem",
  fontFamily: "var(--font-mono)",
  padding: "0.2rem 0.5rem",
  border: "1px solid var(--color-border)",
  borderRadius: "var(--radius-xs)",
  color: "var(--color-fg-muted)",
  textDecoration: "none",
  letterSpacing: "0.05em",
} as const;

function describeReason(status: InstanceStatus): string {
  if (status.crashed_since_start) {
    return "process exited unexpectedly";
  }
  if (!status.running && status.pid > 0) {
    return "process gone, no exit record";
  }
  if (status.health === "unhealthy") {
    return "health probe unhealthy";
  }
  return "diagnostics";
}
