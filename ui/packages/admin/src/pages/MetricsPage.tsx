import { useQuery } from "@tanstack/react-query";
import { PageHeading, Spinner } from "@sockerless/ui-core/components";
import { AdminApiClient, type AdminComponent } from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

function ComponentMetricsPanel({ component }: { component: AdminComponent }) {
  const { data, isError, error } = useQuery({
    queryKey: ["component-metrics", component.name],
    queryFn: () => api.componentMetrics(component.name),
    enabled: component.health === "up",
    refetchInterval: component.health === "up" ? 5000 : false,
  });

  return (
    <div
      className="px-4 py-4"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderLeft:
          component.health === "up"
            ? "3px solid var(--color-accent)"
            : "3px solid var(--color-status-neutral)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div
        className="mb-1 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {component.type}
      </div>
      <h3
        className="mb-3 font-display"
        style={{
          fontStyle: "italic",
          fontWeight: 600,
          fontSize: "1.2rem",
          letterSpacing: "-0.02em",
          lineHeight: 1.1,
          color: "var(--color-fg)",
        }}
      >
        {component.name}
      </h3>
      {isError ? (
        <ErrorPanel message={error?.message ?? "metrics unavailable"} />
      ) : data ? (
        <pre
          className="font-mono"
          style={{
            background: "var(--color-bg-subtle)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-sm)",
            padding: "0.75rem",
            fontSize: "0.7rem",
            maxHeight: "16rem",
            overflow: "auto",
            color: "var(--color-fg)",
            margin: 0,
          }}
        >
          {JSON.stringify(data, null, 2)}
        </pre>
      ) : (
        <div
          className="font-mono uppercase tracking-[0.18em]"
          style={{
            color: "var(--color-fg-subtle)",
            fontSize: "0.7rem",
          }}
        >
          {component.health === "up" ? "loading…" : "— offline —"}
        </div>
      )}
    </div>
  );
}

export function MetricsPage() {
  const {
    data: components,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
  });

  if (isLoading) return <Spinner label="loading components" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!components) return <Spinner label="loading components" />;

  return (
    <div>
      <PageHeading
        kicker="admin · metrics"
        title={<>Per-component metrics</>}
        meta={`${components.length} component${components.length === 1 ? "" : "s"}`}
      />
      {components.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no components registered —
        </p>
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {components.map((c) => (
            <ComponentMetricsPanel key={c.name} component={c} />
          ))}
        </div>
      )}
    </div>
  );
}
