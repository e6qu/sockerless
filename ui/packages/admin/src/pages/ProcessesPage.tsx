import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { Link } from "react-router";
import { AdminApiClient, type ProcessInfo } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

function statusLabel(status: string): string {
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

  if (isLoading) return <Spinner label="loading processes" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!processes) return <Spinner label="loading processes" />;

  return (
    <div>
      <PageHeading
        kicker="admin · processes"
        title={<>Managed processes</>}
        meta={`${processes.length} process${processes.length === 1 ? "" : "es"} configured`}
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

      {processes.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no managed processes (add to admin.json) —
        </p>
      ) : (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-sm)",
            overflow: "hidden",
          }}
        >
          <div className="overflow-x-auto">
            <table
              className="min-w-full font-mono"
              style={{ fontSize: "0.78rem", borderCollapse: "collapse" }}
            >
              <thead style={{ background: "var(--color-bg-subtle)" }}>
                <tr>
                  <Th>Name</Th>
                  <Th>Binary</Th>
                  <Th>Status</Th>
                  <Th>PID</Th>
                  <Th>Address</Th>
                  <Th>Uptime</Th>
                  <Th>Actions</Th>
                </tr>
              </thead>
              <tbody>
                {processes.map((proc: ProcessInfo, i) => (
                  <tr
                    key={proc.name}
                    style={{
                      background:
                        i % 2 === 0
                          ? "var(--color-surface)"
                          : "var(--color-bg-subtle)",
                      borderBottom:
                        "1px solid color-mix(in oklch, var(--color-border) 60%, transparent)",
                    }}
                  >
                    <Td>
                      <Link
                        to={`/ui/processes/${encodeURIComponent(proc.name)}`}
                        style={{
                          color: "var(--color-accent)",
                          fontWeight: 500,
                          textDecoration: "none",
                        }}
                      >
                        {proc.name}
                      </Link>
                    </Td>
                    <Td muted>{proc.binary}</Td>
                    <Td>
                      <StatusBadge status={statusLabel(proc.status)} />
                    </Td>
                    <Td muted>{proc.pid || "—"}</Td>
                    <Td muted>{proc.addr || "—"}</Td>
                    <Td muted>{proc.started_at ? formatUptime(proc.started_at) : "—"}</Td>
                    <Td>
                      {proc.status === "running" ? (
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => stop.mutate(proc.name)}
                          disabled={
                            stop.isPending && stop.variables === proc.name
                          }
                        >
                          Stop
                        </Button>
                      ) : (
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={() => start.mutate(proc.name)}
                          disabled={
                            (start.isPending && start.variables === proc.name) ||
                            proc.status === "starting" ||
                            proc.status === "stopping"
                          }
                        >
                          Start
                        </Button>
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function Th({ children }: { children: React.ReactNode }) {
  return (
    <th
      className="px-3 py-2 text-left uppercase tracking-[0.15em]"
      style={{
        fontSize: "0.62rem",
        fontWeight: 500,
        color: "var(--color-fg-subtle)",
        borderBottom: "1px solid var(--color-border)",
        whiteSpace: "nowrap",
      }}
    >
      {children}
    </th>
  );
}

function Td({ children, muted }: { children: React.ReactNode; muted?: boolean }) {
  return (
    <td
      className="px-3 py-1.5"
      style={{
        whiteSpace: "nowrap",
        color: muted ? "var(--color-fg-muted)" : "var(--color-fg)",
      }}
    >
      {children}
    </td>
  );
}

function formatUptime(startedAt: string): string {
  const start = new Date(startedAt);
  const seconds = Math.floor((Date.now() - start.getTime()) / 1000);
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}
