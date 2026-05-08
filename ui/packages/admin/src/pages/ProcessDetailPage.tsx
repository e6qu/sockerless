import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  LogViewer,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

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

  if (isLoading) return <Spinner label="loading process" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!processes) return <Spinner label="loading process" />;
  if (!proc) {
    return (
      <ErrorPanel
        kicker="not found"
        message={`process "${name ?? ""}" is not registered`}
      />
    );
  }

  const statusStr =
    proc.status === "running"
      ? "running"
      : proc.status === "failed"
        ? "error"
        : proc.status;

  const running = proc.status === "running";

  return (
    <div>
      <PageHeading
        kicker="admin · process"
        title={proc.name}
        meta={
          <span className="inline-flex items-center gap-3">
            <StatusBadge status={statusStr} />
            <span>{proc.binary}</span>
          </span>
        }
        actions={
          running ? (
            <Button
              variant="danger"
              size="sm"
              onClick={() => stop.mutate()}
              disabled={stop.isPending}
            >
              {stop.isPending ? "Stopping…" : "Stop"}
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              onClick={() => start.mutate()}
              disabled={
                start.isPending ||
                proc.status === "starting" ||
                proc.status === "stopping"
              }
            >
              {start.isPending ? "Starting…" : "Start"}
            </Button>
          )
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
      {proc.exit_code !== 0 && proc.status !== "running" && (
        <div className="mb-3">
          <ErrorPanel
            kicker="last exit"
            message={`process exited with code ${proc.exit_code}`}
          />
        </div>
      )}

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MetricsCard title="Binary" value={proc.binary} />
        <MetricsCard title="PID" value={proc.pid || "—"} />
        <MetricsCard title="Address" value={proc.addr || "—"} />
        <MetricsCard
          title="Started"
          value={
            proc.started_at
              ? new Date(proc.started_at).toLocaleTimeString()
              : "—"
          }
        />
      </div>

      <h3
        className="mb-2 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        Logs (tail 200)
      </h3>
      <LogViewer lines={logs ?? []} />
    </div>
  );
}
