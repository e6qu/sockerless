import { Spinner } from "@sockerless/ui-core/components";
import { useMetricsData } from "../hooks/useMetricsData.js";
import { formatUptime } from "../utils/format.js";
import { PageTitle, StatCard, SectionLabel } from "../components/ui.js";

export function MetricsPage() {
  const { metrics, status, isLoading } = useMetricsData();

  if (isLoading && !metrics) return <Spinner label="loading metrics" />;

  return (
    <div>
      <PageTitle
        title="Runtime & throughput"
        meta={
          metrics
            ? `uptime ${formatUptime(metrics.uptime_seconds)} · ${metrics.goroutines} goroutines · ${metrics.heap_alloc_mb.toFixed(1)} MB heap`
            : undefined
        }
      />

      {metrics && (
        <section className="mb-8">
          <SectionLabel>Counters</SectionLabel>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard title="Workflow submissions" value={metrics.workflow_submissions} />
            <StatCard title="Job dispatches" value={metrics.job_dispatches} />
            <StatCard
              title="Active workflows"
              value={metrics.active_workflows}
              emphasized={metrics.active_workflows > 0}
            />
            <StatCard
              title="Active sessions"
              value={metrics.active_sessions}
              emphasized={metrics.active_sessions > 0}
            />
          </div>
        </section>
      )}

      {status && (
        <section className="mb-8">
          <SectionLabel>Jobs by status</SectionLabel>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Object.keys(status.jobs_by_status).length === 0 ? (
              <EmptyCell>no jobs in flight</EmptyCell>
            ) : (
              Object.entries(status.jobs_by_status).map(([s, count]) => (
                <StatCard key={s} title={s} value={count} emphasized={s === "running" || s === "queued"} />
              ))
            )}
          </div>
        </section>
      )}

      {metrics && (
        <section>
          <SectionLabel>Job completions</SectionLabel>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Object.keys(metrics.job_completions).length === 0 ? (
              <EmptyCell>no completed jobs yet</EmptyCell>
            ) : (
              Object.entries(metrics.job_completions).map(([result, count]) => (
                <StatCard key={result} title={result} value={count} emphasized={result === "failure"} />
              ))
            )}
          </div>
        </section>
      )}
    </div>
  );
}

function EmptyCell({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="col-span-full"
      style={{
        padding: "1.25rem",
        textAlign: "center",
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        color: "var(--color-fg-muted)",
        fontSize: "0.85rem",
      }}
    >
      {children}
    </div>
  );
}
