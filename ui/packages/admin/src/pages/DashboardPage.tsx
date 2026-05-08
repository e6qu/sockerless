import { useQuery } from "@tanstack/react-query";
import {
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { AdminApiClient } from "../api.js";

const api = new AdminApiClient();

export function DashboardPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["overview"],
    queryFn: () => api.overview(),
  });

  if (isLoading) return <Spinner label="loading overview" />;
  if (isError) {
    return (
      <div
        className="px-4 py-3 font-mono"
        style={{
          background: "var(--color-status-error-soft)",
          color: "var(--color-status-error)",
          border: "1px solid var(--color-status-error)",
          borderLeft: "3px solid var(--color-status-error)",
          borderRadius: "var(--radius-sm)",
          fontSize: "0.78rem",
        }}
      >
        error: {error?.message ?? "failed to load"}
      </div>
    );
  }
  if (!data) return <Spinner label="loading overview" />;

  const downCount = data.components_down ?? 0;

  return (
    <div>
      <PageHeading
        kicker="cluster · health"
        title={<>System overview</>}
        meta={`${data.components.length} components · ${data.backends} backends · ${data.total_containers} containers`}
      />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 mb-8">
        <MetricsCard
          title="Components up"
          value={data.components_up}
          emphasized={data.components_up > 0}
        />
        <MetricsCard
          title="Components down"
          value={downCount}
          emphasized={downCount > 0}
        />
        <MetricsCard title="Backends" value={data.backends} />
        <MetricsCard title="Containers" value={data.total_containers} />
      </div>

      <h3
        className="mb-3 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        Component health
      </h3>
      {data.components.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no components registered —
        </p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {data.components.map((c, i) => (
            <div
              key={c.name}
              className="reveal flex items-center justify-between px-4 py-3"
              style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                "--reveal-delay": `${i * 30}ms`,
              } as React.CSSProperties}
            >
              <div className="min-w-0">
                <p
                  className="truncate font-mono"
                  style={{
                    fontSize: "0.85rem",
                    fontWeight: 500,
                    color: "var(--color-fg)",
                  }}
                  title={c.name}
                >
                  {c.name}
                </p>
                <p
                  className="mt-0.5 text-[11px] font-mono"
                  style={{ color: "var(--color-fg-subtle)" }}
                >
                  {c.type} · {c.addr}
                </p>
              </div>
              <StatusBadge
                status={
                  c.health === "up"
                    ? "ok"
                    : c.health === "unknown"
                      ? "warning"
                      : "error"
                }
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
