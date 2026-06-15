import { useQuery } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router";
import {
  PageHeading,
  StatusBadge,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import { api } from "../api.js";
import type { Job } from "../types.js";
import { shortSHA, bytes } from "../format.js";

export function PipelineDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const pipeline = useQuery({
    queryKey: ["pipeline", id],
    queryFn: () => api.pipeline(id!),
  });

  if (pipeline.isLoading) return <Spinner />;
  if (pipeline.error) {
    return <InlineError title={`Failed to load pipeline ${id}`} detail={pipeline.error as Error} />;
  }
  const pl = pipeline.data!;
  const jobs = pl.job_list ?? [];
  const stages = pl.stages.length ? pl.stages : [...new Set(jobs.map((j) => j.stage))];

  return (
    <div>
      <PageHeading
        kicker={`Pipeline #${pl.id}`}
        title={pl.project_name}
        meta={`${pl.ref} · ${shortSHA(pl.sha)}`}
        actions={<StatusBadge status={pl.status} />}
      />

      {/* GitLab-style stage graph: one column per stage, job cards stacked. */}
      <div className="flex gap-4 overflow-x-auto pb-4">
        {stages.map((stage) => (
          <div key={stage} className="min-w-[200px] flex-1">
            <div
              className="mb-2 text-[11px] font-semibold uppercase tracking-wide"
              style={{ color: "var(--color-fg-subtle)" }}
            >
              {stage}
            </div>
            <div className="flex flex-col gap-2">
              {jobs
                .filter((j) => j.stage === stage)
                .map((j) => (
                  <JobCard key={j.id} job={j} onClick={() => navigate(`/ui/jobs/${j.id}`)} />
                ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function JobCard({ job, onClick }: { job: Job; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex flex-col items-start gap-1.5 px-3 py-2 text-left"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderLeft: "3px solid var(--color-accent)",
        borderRadius: "var(--radius-sm)",
        cursor: "pointer",
      }}
    >
      <span className="text-sm font-medium" style={{ color: "var(--color-fg)" }}>
        {job.name}
      </span>
      <StatusBadge status={job.status} />
      {job.artifact_size > 0 && (
        <span className="text-[11px]" style={{ color: "var(--gl-orange-strong)" }}>
          artifact {bytes(job.artifact_size)}
        </span>
      )}
    </button>
  );
}
