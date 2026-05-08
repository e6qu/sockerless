import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  LogViewer,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchWorkflowDetail, fetchWorkflowLogs } from "../api.js";
import type { BleephubWorkflowJob } from "../types.js";

const col = createColumnHelper<BleephubWorkflowJob>();

export function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: wf, isLoading, isError } = useQuery({
    queryKey: ["workflow", id],
    queryFn: () => fetchWorkflowDetail(id!),
    enabled: !!id,
    refetchInterval: 3000,
  });
  const { data: logs } = useQuery({
    queryKey: ["workflow-logs", id],
    queryFn: () => fetchWorkflowLogs(id!),
    enabled: !!id,
    refetchInterval: 5000,
  });

  if (!id) {
    return (
      <div
        className="px-4 py-3 font-mono"
        style={{
          background: "var(--color-status-error-soft)",
          color: "var(--color-status-error)",
          border: "1px solid var(--color-status-error)",
          borderRadius: "var(--radius-sm)",
          fontSize: "0.78rem",
        }}
      >
        missing workflow id in route
      </div>
    );
  }
  if (isLoading) return <Spinner label="loading workflow" />;
  if (isError || !wf) {
    return (
      <div
        className="px-4 py-3 font-mono"
        style={{
          background: "var(--color-status-error-soft)",
          color: "var(--color-status-error)",
          border: "1px solid var(--color-status-error)",
          borderRadius: "var(--radius-sm)",
          fontSize: "0.78rem",
        }}
      >
        workflow {id} not found or fetch failed
      </div>
    );
  }

  const jobs = Object.values(wf.jobs).sort((a, b) => a.key.localeCompare(b.key));

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columns: any[] = [
    col.accessor("key", {
      header: "Key",
      cell: (info) => (
        <span
          className="font-mono"
          style={{ color: "var(--color-accent)" }}
        >
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("displayName", {
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
    col.accessor("needs", {
      header: "Needs",
      cell: (info) => (
        <span style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()?.join(", ") || "—"}
        </span>
      ),
    }),
    col.accessor("matrix", {
      header: "Matrix",
      cell: (info) => {
        const m = info.getValue();
        if (!m) return <span style={{ color: "var(--color-fg-subtle)" }}>—</span>;
        return (
          <span style={{ color: "var(--color-fg-muted)" }}>
            {Object.entries(m)
              .map(([k, v]) => `${k}=${v}`)
              .join(", ")}
          </span>
        );
      },
    }),
  ];

  return (
    <div>
      <PageHeading
        kicker={`workflow · run #${wf.runId}`}
        title={wf.name}
        meta={
          <span className="inline-flex flex-wrap items-center gap-3">
            <StatusBadge status={wf.status} />
            {wf.result && <StatusBadge status={wf.result} />}
            {wf.eventName && <span>event: {wf.eventName}</span>}
            {wf.repoFullName && <span>repo: {wf.repoFullName}</span>}
            <span>{new Date(wf.createdAt).toLocaleString()}</span>
          </span>
        }
      />

      <h3
        className="mb-3 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        Jobs ({jobs.length})
      </h3>
      <DataTable
        data={jobs}
        columns={columns}
        filterPlaceholder="Filter jobs…"
        emptyMessage="No jobs in this workflow."
      />

      {logs && Object.keys(logs).length > 0 && (
        <section className="mt-8">
          <h3
            className="mb-3 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Logs
          </h3>
          <div className="space-y-4">
            {jobs.map((job) => {
              const jobLogs = logs[job.jobId];
              if (!jobLogs || jobLogs.length === 0) return null;
              return (
                <div key={job.jobId}>
                  <p
                    className="mb-1 font-mono"
                    style={{
                      color: "var(--color-fg)",
                      fontSize: "0.8rem",
                      fontWeight: 500,
                    }}
                  >
                    {job.displayName}{" "}
                    <span
                      className="font-mono"
                      style={{
                        color: "var(--color-fg-subtle)",
                        fontSize: "0.7rem",
                      }}
                    >
                      ({job.key})
                    </span>
                  </p>
                  <LogViewer lines={jobLogs} />
                </div>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}
