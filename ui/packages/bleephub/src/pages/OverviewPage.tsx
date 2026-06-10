import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  InlineError,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { fetchHealth, fetchMetrics, fetchWorkflows, fetchStorageInfo } from "../api.js";
import type { BleephubWorkflow } from "../types.js";
import { formatUptime } from "../utils/format.js";

const col = createColumnHelper<BleephubWorkflow>();

export function OverviewPage() {
  const navigate = useNavigate();
  const { data: health } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    refetchInterval: 5000,
  });
  const { data: metrics, isLoading, isError } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 3000,
  });
  const { data: workflows } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });
  const { data: storageInfo } = useQuery({
    queryKey: ["storage"],
    queryFn: fetchStorageInfo,
    refetchInterval: 30000,
  });

  if (isError) return <InlineError title="Failed to load overview" />;
  if (isLoading || !metrics) return <Spinner label="loading overview" />;

  const recent = (workflows ?? []).slice(0, 10);

  const columns = [
    col.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("status", {
      header: "Status",
      cell: (info) => <StatusBadge status={info.getValue()} />,
    }),
    col.accessor("result", {
      header: "Result",
      cell: (info) => {
        const v = info.getValue();
        return v ? <StatusBadge status={v} /> : null;
      },
    }),
    col.accessor("eventName", {
      header: "Event",
      cell: (info) => (
        <span
          className="font-mono uppercase tracking-[0.1em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.65rem" }}
        >
          {info.getValue() ?? "—"}
        </span>
      ),
    }),
    col.display({
      id: "jobs",
      header: "Jobs",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {Object.keys(info.row.original.jobs).length}
        </span>
      ),
    }),
  ];

  return (
    <div>
      <PageHeading
        kicker="bleephub · overview"
        title={<>System status</>}
        meta={
          <span className="inline-flex items-center gap-2">
            {health ? (
              <StatusBadge status={health.status === "ok" ? "ok" : "error"} />
            ) : (
              <span style={{ color: "var(--color-fg-subtle)" }}>health unknown</span>
            )}
            <span>·</span>
            <span>uptime {formatUptime(metrics.uptime_seconds)}</span>
          </span>
        }
      />

      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-5">
        <MetricsCard
          title="Active workflows"
          value={metrics.active_workflows}
          emphasized={metrics.active_workflows > 0}
        />
        <MetricsCard title="Connected runners" value={metrics.active_sessions} />
        <MetricsCard title="Submissions" value={metrics.workflow_submissions} />
        <MetricsCard title="Job dispatches" value={metrics.job_dispatches} />
        <MetricsCard title="Uptime" value={formatUptime(metrics.uptime_seconds)} />
      </div>

      {storageInfo && (
        <div
          className="mb-8 rounded border p-3"
          style={{
            borderColor: "var(--color-border)",
            background: "var(--color-bg-elevated)",
          }}
        >
          <h3
            className="mb-2 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Storage backends
          </h3>
          <div className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
            <div>
              <span style={{ color: "var(--color-fg-subtle)" }}>Persistence</span>
              <span className="ml-2 font-mono" style={{ color: "var(--color-fg)" }}>
                {storageInfo.persistence === "none" ? "none (in-memory)" : storageInfo.dialect}
              </span>
            </div>
            <div>
              <span style={{ color: "var(--color-fg-subtle)" }}>Git storage</span>
              <span className="ml-2 font-mono" style={{ color: "var(--color-fg)" }}>
                {storageInfo.git === "memory" ? "memory (ephemeral)" : storageInfo.git}
                {storageInfo.git === "filesystem" && storageInfo.git_details.dir
                  ? ` (${storageInfo.git_details.dir})`
                  : ""}
                {storageInfo.git === "s3" && storageInfo.git_details.bucket
                  ? ` (${storageInfo.git_details.bucket})`
                  : ""}
              </span>
            </div>
          </div>
        </div>
      )}

      <h3
        className="mb-3 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        Recent workflows
      </h3>
      <DataTable
        data={recent}
        columns={columns}
        filterPlaceholder="Filter recent workflows…"
        emptyMessage="No workflow runs yet."
        onRowClick={(row) => navigate(`/ui/workflows/${row.id}`)}
      />
    </div>
  );
}
