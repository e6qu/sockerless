import { useQueries } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { fetchHealth, fetchStatus, fetchStorageInfo } from "../api.js";
import { formatUptime } from "../utils/format.js";
import { Box, PageTitle, SectionLabel, StatCard } from "../components/ui.js";

export function StorageHealthPage() {
  const [
    { data: storage, isLoading: sLoading, isError: sError },
    { data: status, isLoading: stLoading, isError: stError },
    { data: health, isLoading: hLoading, isError: hError },
  ] = useQueries({
    queries: [
      { queryKey: ["storage"], queryFn: fetchStorageInfo },
      { queryKey: ["status"], queryFn: fetchStatus },
      { queryKey: ["health"], queryFn: fetchHealth },
    ],
  });

  if (sError || stError || hError) return <InlineError title="Failed to load storage/health" />;
  if (sLoading || stLoading || hLoading || !storage || !status || !health) {
    return <Spinner label="loading storage health" />;
  }

  return (
    <div>
      <PageTitle title="Storage & health" meta="Persistence, runtime status, and health." />

      <section className="mb-8">
        <SectionLabel>Health</SectionLabel>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatCard title="Service" value={health.service} />
          <StatCard title="Status" value={health.status} />
          <StatCard title="Uptime" value={formatUptime(status.uptime_seconds)} />
          <StatCard title="Active workflows" value={status.active_workflows} />
        </div>
      </section>

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

      <section className="mb-8">
        <SectionLabel>Storage backend</SectionLabel>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatCard title="Persistence" value={storage.persistence} />
          <StatCard title="Dialect" value={storage.dialect} />
          <StatCard title="Git storage" value={storage.git} />
        </div>
      </section>

      {storage.git_details && Object.keys(storage.git_details).length > 0 && (
        <section>
          <SectionLabel>Git details</SectionLabel>
          <Box>
            <dl
              style={{
                display: "grid",
                gridTemplateColumns: "minmax(120px, auto) 1fr",
                gap: "0.5rem 1rem",
                padding: "1rem",
                fontSize: "0.85rem",
                margin: 0,
              }}
            >
              {Object.entries(storage.git_details).map(([k, v]) => (
                <>
                  <dt style={{ color: "var(--color-fg-muted)", fontWeight: 500 }}>{k}</dt>
                  <dd style={{ color: "var(--color-fg)", margin: 0 }}>{v}</dd>
                </>
              ))}
            </dl>
          </Box>
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
