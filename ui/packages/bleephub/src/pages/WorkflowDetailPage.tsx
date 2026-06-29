import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, LogViewer, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchWorkflowDetail, fetchWorkflowLogs } from "../api.js";
import type { BleephubWorkflowJob } from "../types.js";
import { PageTitle, SectionLabel } from "../components/ui.js";

const col = createColumnHelper<BleephubWorkflowJob>();

export function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: wf, isLoading, isError } = useQuery({
    queryKey: ["workflow", id],
    queryFn: () => fetchWorkflowDetail(id!),
    enabled: !!id,
    refetchInterval: 3000,
  });
  const {
    data: logs,
    isError: logsIsError,
    error: logsError,
  } = useQuery({
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

  const columns = [
    col.accessor("displayName", {
      header: "Job",
      cell: (info) => {
        const job = info.row.original;
        const name = info.getValue();
        return (
          <span className="inline-flex items-center gap-2">
            <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>{name}</span>
            {job.key !== name && (
              <span
                className="font-mono"
                style={{ color: "var(--color-accent)", fontSize: "0.75em" }}
              >
                {job.key}
              </span>
            )}
          </span>
        );
      },
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
      <PageTitle
        title={
          <>
            {wf.name} <span style={{ color: "var(--color-fg-muted)", fontWeight: 400 }}>#{wf.runId}</span>
          </>
        }
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

      <SectionLabel>Jobs ({jobs.length})</SectionLabel>
      <DataTable
        data={jobs}
        columns={columns}
        filterPlaceholder="Filter jobs…"
        emptyMessage="No jobs in this workflow."
      />

      {logsIsError && (
        // A failed log fetch must not render as "no logs exist" — say so.
        <section className="mt-8">
          <SectionLabel>Logs</SectionLabel>
          <InlineError
            inline
            title="Failed to load logs"
            detail={logsError instanceof Error ? logsError.message : String(logsError)}
          />
        </section>
      )}

      {!logsIsError && logs && Object.keys(logs).length > 0 && (
        <section className="mt-8">
          <SectionLabel>Logs</SectionLabel>
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
