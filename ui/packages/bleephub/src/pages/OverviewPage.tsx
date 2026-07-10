import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { fetchHealth, fetchMetrics, fetchWorkflows } from "../api.js";
import type { BleephubWorkflow } from "../types.js";
import { PageTitle, StatCard, SectionLabel } from "../components/ui.js";

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
  const { data: workflows, isError: workflowsError } = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
    refetchInterval: 3000,
  });
  if (isError) return <InlineError title="Failed to load overview" />;
  if (isLoading || !metrics) return <Spinner label="loading overview" />;

  const recent = (workflows ?? []).slice(0, 10);

  const columns = [
    col.accessor("name", {
      header: "Name",
      cell: (info) => (
        <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{info.getValue()}</span>
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
        <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue() ?? "—"}</span>
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
      <PageTitle
        title="System status"
        meta={
          <span className="inline-flex items-center gap-2">
            {health ? (
              <StatusBadge status={health.status === "ok" ? "ok" : "error"} />
            ) : (
              <span style={{ color: "var(--color-fg-subtle)" }}>health unknown</span>
            )}
          </span>
        }
      />

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-5">
        <StatCard
          title="Active workflows"
          value={metrics.active_workflows}
          emphasized={metrics.active_workflows > 0}
        />
        <StatCard title="Connected runners" value={metrics.connected_runners} />
        <StatCard title="Workflow runs" value={metrics.workflow_runs} />
        <StatCard title="Job dispatches" value={metrics.job_dispatches} />
      </div>

      <SectionLabel>Recent workflows</SectionLabel>
      {workflowsError ? (
        <InlineError title="Failed to load workflows" />
      ) : (
        <DataTable
          data={recent}
          columns={columns}
          filterPlaceholder="Filter recent workflows…"
          emptyMessage="No workflow runs yet."
          onRowClick={(row) => navigate(`/ui/workflows/${row.id}`)}
        />
      )}
    </div>
  );
}
