import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { fetchHealth, fetchMetrics, fetchWorkflows } from "../api.js";
import type { BleephubWorkflow } from "../types.js";

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

const col = createColumnHelper<BleephubWorkflow>();

export function OverviewPage() {
  const navigate = useNavigate();
  const { data: health } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    refetchInterval: 5000,
  });
  const { data: metrics, isLoading } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 3000,
  });
  const { data: workflows } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });

  if (isLoading || !metrics) return <Spinner label="loading overview" />;

  const recent = (workflows ?? []).slice(0, 10);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
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
