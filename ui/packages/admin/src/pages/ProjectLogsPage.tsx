import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  LogViewer,
  PageHeading,
  Spinner,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

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

  const {
    data: logs,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["project-logs", name, component],
    queryFn: () => api.projectLogs(name!, component || undefined, 200),
    enabled: !!name,
    refetchInterval: 2000,
  });

  if (!name) {
    return <ErrorPanel message="missing project name in route" />;
  }

  return (
    <div>
      <PageHeading
        kicker={`admin · project · ${name}`}
        title={<>Logs</>}
        meta={
          <span className="inline-flex items-center gap-3">
            <Link
              to={`/ui/projects/${encodeURIComponent(name)}`}
              style={{
                color: "var(--color-accent)",
                textDecoration: "none",
              }}
            >
              ← back to project
            </Link>
            <span>tail 200</span>
          </span>
        }
      />

      <div
        className="mb-4 inline-flex"
        style={{
          background: "var(--color-bg-subtle)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-sm)",
          padding: "2px",
        }}
      >
        {components.map((c) => {
          const active = component === c.value;
          return (
            <button
              key={c.value}
              type="button"
              onClick={() => setComponent(c.value)}
              className="px-3 py-1 font-mono uppercase tracking-[0.12em]"
              style={{
                fontSize: "0.7rem",
                background: active ? "var(--color-accent)" : "transparent",
                color: active ? "var(--color-accent-fg)" : "var(--color-fg-muted)",
                border: 0,
                borderRadius: "2px",
                transition: "all 0.12s var(--ease-out-quint)",
              }}
            >
              {c.label}
            </button>
          );
        })}
      </div>

      {isLoading ? (
        <Spinner label="loading logs" />
      ) : isError ? (
        <ErrorPanel message={error?.message} />
      ) : (
        <LogViewer lines={logs ?? []} />
      )}
    </div>
  );
}
