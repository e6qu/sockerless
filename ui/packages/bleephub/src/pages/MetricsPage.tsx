import { useQuery } from "@tanstack/react-query";
import { MetricsCard, PageHeading, Spinner } from "@sockerless/ui-core/components";
import { fetchMetrics, fetchStatus } from "../api.js";

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function MetricsPage() {
  const { data: metrics, isLoading: metricsLoading } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5000,
  });
  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["status"],
    queryFn: fetchStatus,
    refetchInterval: 5000,
  });

  if ((metricsLoading && !metrics) || (statusLoading && !status)) {
    return <Spinner label="loading metrics" />;
  }

  return (
    <div>
      <PageHeading
        kicker="bleephub · metrics"
        title={<>Runtime &amp; throughput</>}
        meta={metrics ? `uptime ${formatUptime(metrics.uptime_seconds)} · ${metrics.goroutines} goroutines · ${metrics.heap_alloc_mb.toFixed(1)} MB heap` : undefined}
      />

      {metrics && (
        <>
          <SectionHeading>Counters</SectionHeading>
          <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <MetricsCard title="Workflow submissions" value={metrics.workflow_submissions} />
            <MetricsCard title="Job dispatches" value={metrics.job_dispatches} />
            <MetricsCard
              title="Active workflows"
              value={metrics.active_workflows}
              emphasized={metrics.active_workflows > 0}
            />
            <MetricsCard
              title="Active sessions"
              value={metrics.active_sessions}
              emphasized={metrics.active_sessions > 0}
            />
          </div>
        </>
      )}

      {status && (
        <>
          <SectionHeading>Jobs by status</SectionHeading>
          <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Object.keys(status.jobs_by_status).length === 0 ? (
              <EmptyCell>no jobs in flight</EmptyCell>
            ) : (
              Object.entries(status.jobs_by_status).map(([s, count]) => (
                <MetricsCard
                  key={s}
                  title={s}
                  value={count}
                  emphasized={s === "running" || s === "queued"}
                />
              ))
            )}
          </div>
        </>
      )}

      {metrics && (
        <>
          <SectionHeading>Job completions</SectionHeading>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Object.keys(metrics.job_completions).length === 0 ? (
              <EmptyCell>no completed jobs yet</EmptyCell>
            ) : (
              Object.entries(metrics.job_completions).map(([result, count]) => (
                <MetricsCard
                  key={result}
                  title={result}
                  value={count}
                  emphasized={result === "Failed" || result === "failed"}
                />
              ))
            )}
          </div>
        </>
      )}
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h3
      className="mb-3 text-[10px] uppercase tracking-[0.22em]"
      style={{ color: "var(--color-fg-subtle)" }}
    >
      {children}
    </h3>
  );
}

function EmptyCell({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="col-span-full px-4 py-6 text-center font-mono uppercase tracking-[0.18em]"
      style={{
        background: "var(--color-bg-subtle)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
        color: "var(--color-fg-subtle)",
        fontSize: "0.7rem",
      }}
    >
      — {children} —
    </div>
  );
}
